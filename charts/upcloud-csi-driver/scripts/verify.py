#!/usr/bin/env python3
"""Lint the chart and compare it with the static Kubernetes manifests."""

import copy
import subprocess
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    sys.stderr.write("PyYAML is required: pip install pyyaml\n")
    sys.exit(1)

ROOT = Path(__file__).resolve().parents[3]
CHART = ROOT / "charts" / "upcloud-csi-driver"
STATIC = [
    ROOT / "deploy" / "kubernetes" / "rbac-upcloud-csi.yaml",
    ROOT / "deploy" / "kubernetes" / "setup-upcloud-csi.yaml",
]
CRD_SOURCE = ROOT / "deploy" / "kubernetes" / "crd-upcloud-csi.yaml"

HELM_LABELS = {
    "helm.sh/chart",
    "app.kubernetes.io/name",
    "app.kubernetes.io/instance",
    "app.kubernetes.io/managed-by",
}

# kubectl drops metadata.namespace on these kinds. The chart omits it.
CLUSTER_SCOPED = {
    "ClusterRole",
    "ClusterRoleBinding",
    "CSIDriver",
    "CustomResourceDefinition",
    "StorageClass",
    "VolumeSnapshotClass",
}


def run(cmd, echo_stdout=True):
    print("+", " ".join(cmd), flush=True)
    result = subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True)
    if echo_stdout and result.stdout:
        print(result.stdout, end="" if result.stdout.endswith("\n") else "\n")
    if result.returncode != 0:
        if not echo_stdout and result.stdout:
            print(result.stdout, end="" if result.stdout.endswith("\n") else "\n")
        sys.stderr.write(result.stderr)
        sys.exit(result.returncode)
    if result.stderr:
        sys.stderr.write(result.stderr)
    return result.stdout


def load_docs(text):
    docs = []
    for doc in yaml.safe_load_all(text):
        if isinstance(doc, dict) and doc.get("kind"):
            docs.append(doc)
    return docs


def load_file(path):
    return load_docs(path.read_text())


def normalize(obj):
    if isinstance(obj, dict):
        out = {}
        for key, value in obj.items():
            if key == "labels" and isinstance(value, dict):
                value = {k: v for k, v in value.items() if k not in HELM_LABELS}
                if not value:
                    continue
            out[key] = normalize(value)
        return out
    if isinstance(obj, list):
        return [normalize(item) for item in obj]
    return obj


def identity(doc):
    return (doc.get("kind"), doc["metadata"]["name"])


def index(docs):
    found = {}
    for doc in docs:
        key = identity(doc)
        if key in found:
            raise SystemExit(f"duplicate resource {key[0]}/{key[1]}")
        normalized = normalize(doc)
        if normalized.get("kind") in CLUSTER_SCOPED:
            metadata = normalized.get("metadata")
            if isinstance(metadata, dict):
                metadata.pop("namespace", None)
        found[key] = normalized
    return found


def diff(rendered, manifest, path="$"):
    mismatches = []
    if type(rendered) is not type(manifest) and not (
        isinstance(rendered, (int, float)) and isinstance(manifest, (int, float))
    ):
        mismatches.append(f"{path}: rendered {rendered!r} != manifest {manifest!r}")
        return mismatches
    if isinstance(rendered, dict):
        for key in sorted(set(rendered) | set(manifest)):
            child = f"{path}.{key}"
            if key not in rendered:
                mismatches.append(f"{child}: missing from chart, manifest has {manifest[key]!r}")
            elif key not in manifest:
                mismatches.append(f"{child}: extra in chart {rendered[key]!r}")
            else:
                mismatches.extend(diff(rendered[key], manifest[key], child))
        return mismatches
    if isinstance(rendered, list):
        if len(rendered) != len(manifest):
            mismatches.append(
                f"{path}: length {len(rendered)} != {len(manifest)}\n"
                f"  chart:    {rendered!r}\n"
                f"  manifest: {manifest!r}"
            )
            return mismatches
        for i, (left, right) in enumerate(zip(rendered, manifest)):
            mismatches.extend(diff(left, right, f"{path}[{i}]"))
        return mismatches
    if rendered != manifest:
        mismatches.append(f"{path}: rendered {rendered!r} != manifest {manifest!r}")
    return mismatches


def helm_template(namespace, sets=None):
    cmd = ["helm", "template", "upcloud-csi", str(CHART), "--namespace", namespace]
    for item in sets or []:
        cmd.extend(["--set", item])
    return load_docs(run(cmd, echo_stdout=False))


def find(docs, kind, name):
    for doc in docs:
        if doc.get("kind") == kind and doc["metadata"]["name"] == name:
            return doc
    raise SystemExit(f"rendered chart is missing {kind}/{name}")


def compare(rendered_docs, manifest_docs, skip=frozenset()):
    rendered = index(rendered_docs)
    manifest = index(manifest_docs)
    rendered = {key: value for key, value in rendered.items() if key not in skip}
    manifest = {key: value for key, value in manifest.items() if key not in skip}
    ok = True
    for key in sorted(set(rendered) | set(manifest)):
        label = f"{key[0]}/{key[1]}"
        if key not in rendered:
            print(f"MISSING {label}")
            ok = False
            continue
        if key not in manifest:
            print(f"EXTRA {label}")
            ok = False
            continue
        mismatches = diff(rendered[key], manifest[key], label)
        if mismatches:
            ok = False
            print(f"DIFF {label}")
            for line in mismatches:
                print(f"  {line}")
    return ok


def strip_crd(doc):
    doc = copy.deepcopy(doc)
    metadata = doc.get("metadata") or {}
    if metadata.get("creationTimestamp") is None:
        metadata.pop("creationTimestamp", None)
    doc.pop("status", None)
    return doc


def compare_crds():
    source = {doc["metadata"]["name"]: strip_crd(doc) for doc in load_file(CRD_SOURCE)}
    chart_docs = []
    for path in sorted((CHART / "crds").glob("*.yaml")):
        chart_docs.extend(load_file(path))
    chart = {doc["metadata"]["name"]: strip_crd(doc) for doc in chart_docs}
    print(f"comparing {len(chart)} snapshot CRDs")
    ok = True
    for name in sorted(set(source) | set(chart)):
        if name not in chart:
            print(f"MISSING CRD {name}")
            ok = False
            continue
        if name not in source:
            print(f"EXTRA CRD {name}")
            ok = False
            continue
        mismatches = diff(chart[name], source[name], name)
        if mismatches:
            ok = False
            print(f"DIFF CRD {name}")
            for line in mismatches:
                print(f"  {line}")
    return ok


def main():
    run(["helm", "lint", str(CHART)])
    rendered = helm_template("kube-system")
    manifest = []
    for path in STATIC:
        manifest.extend(load_file(path))
    print(f"comparing {len(rendered)} rendered resources with the static manifests")
    ok = True
    for doc in rendered:
        metadata = doc.get("metadata") or {}
        if doc.get("kind") in CLUSTER_SCOPED and "namespace" in metadata:
            print(f"{doc['kind']}/{metadata.get('name')} sets metadata.namespace")
            ok = False
    ok = compare(rendered, manifest) and ok
    ok = compare_crds() and ok

    without_classes = helm_template("kube-system", ["storageClasses.enabled=false"])
    if any(doc.get("kind") == "StorageClass" for doc in without_classes):
        print("storageClasses.enabled=false still rendered a StorageClass")
        ok = False
    else:
        ok = compare(
            without_classes,
            manifest,
            skip={
                ("StorageClass", "upcloud-block-storage-maxiops"),
                ("StorageClass", "upcloud-block-storage-hdd"),
                ("StorageClass", "upcloud-block-storage-standard"),
            },
        ) and ok

    without_snap = helm_template("kube-system", ["volumeSnapshotClass.enabled=false"])
    if any(doc.get("kind") == "VolumeSnapshotClass" for doc in without_snap):
        print("volumeSnapshotClass.enabled=false still rendered a VolumeSnapshotClass")
        ok = False
    else:
        ok = compare(
            without_snap,
            manifest,
            skip={("VolumeSnapshotClass", "upcloud-csi-snapshotclass")},
        ) and ok

    custom = helm_template(
        "csi-test",
        [
            "controller.replicas=3",
            "snapshotController.replicas=4",
            "credentials.secretName=my-secret",
            "image.repository=example.com/csi",
            "image.tag=v9.9.9",
            "sidecars.provisioner.tag=v9.9.9",
            "resources.node.plugin.requests.cpu=100m",
            "resources.node.plugin.requests.memory=128Mi",
            "resources.node.plugin.limits.memory=256Mi",
            "resources.controller.provisioner.requests.cpu=50m",
        ],
    )
    controller = find(custom, "Deployment", "csi-upcloud-controller")
    snapshot = find(custom, "Deployment", "csi-upcloud-snapshot-controller")
    node = find(custom, "DaemonSet", "csi-upcloud-node")
    driver = find(custom, "CSIDriver", "storage.csi.upcloud.com")
    storage_class = find(custom, "StorageClass", "upcloud-block-storage-maxiops")
    binding = find(custom, "ClusterRoleBinding", "csi-upcloud-provisioner-binding")

    checks = [
        controller["metadata"]["namespace"] == "csi-test",
        controller["spec"]["replicas"] == 3,
        snapshot["spec"]["replicas"] == 4,
        snapshot["metadata"]["namespace"] == "csi-test",
        "namespace" not in driver.get("metadata", {}),
        "namespace" not in storage_class["metadata"],
        "namespace" not in find(custom, "ClusterRole", "csi-upcloud-node-driver-registrar-role")["metadata"],
        "namespace" not in find(custom, "VolumeSnapshotClass", "upcloud-csi-snapshotclass")["metadata"],
        binding["subjects"][0]["namespace"] == "csi-test",
    ]
    plugin = controller["spec"]["template"]["spec"]["containers"][-1]
    provisioner = controller["spec"]["template"]["spec"]["containers"][0]
    node_plugin = node["spec"]["template"]["spec"]["containers"][-1]
    checks.extend(
        [
            plugin["image"] == "example.com/csi:v9.9.9",
            node_plugin["image"] == "example.com/csi:v9.9.9",
            provisioner["image"].endswith(":v9.9.9"),
            plugin["env"][1]["valueFrom"]["secretKeyRef"]["name"] == "my-secret",
            plugin["env"][2]["valueFrom"]["secretKeyRef"]["name"] == "my-secret",
            node_plugin["env"][1]["valueFrom"]["secretKeyRef"]["name"] == "my-secret",
            node_plugin["resources"]["requests"]["cpu"] == "100m",
            node_plugin["resources"]["requests"]["memory"] == "128Mi",
            node_plugin["resources"]["limits"]["memory"] == "256Mi",
            provisioner["resources"]["requests"]["cpu"] == "50m",
            "resources" not in controller["spec"]["template"]["spec"]["containers"][1],
        ]
    )
    if not all(checks):
        print("custom values were not applied as expected")
        print("plugin", plugin["image"], plugin["env"])
        print("provisioner", provisioner["image"], provisioner.get("resources"))
        print("node", node_plugin["image"], node_plugin.get("resources"))
        print("controller replicas", controller["spec"]["replicas"], controller["metadata"])
        ok = False

    if not ok:
        sys.exit(1)
    print("helm chart matches the static manifests")


if __name__ == "__main__":
    main()
