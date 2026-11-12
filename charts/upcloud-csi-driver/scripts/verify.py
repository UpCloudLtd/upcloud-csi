#!/usr/bin/env python3
"""Lint the chart and compare it with the static Kubernetes manifests."""

import re
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
SHARED_SNAPSHOT_RESOURCES = {
    ("Deployment", "csi-upcloud-snapshot-controller"),
    ("ClusterRole", "csi-upcloud-snapshot-controller-role"),
    ("ClusterRoleBinding", "snapshot-controller-role-binding"),
    ("Role", "csi-upcloud-snapshot-controller-leaderelection-role"),
    ("RoleBinding", "snapshot-controller-leaderelection-binding"),
}
ENCRYPTED_CLASS = ("StorageClass", "upcloud-block-storage-maxiops-encrypted")
ZONE = "fi-hel1"

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


def helm_command(namespace, sets=None):
    cmd = [
        "helm", "template", "upcloud-csi", str(CHART),
        "--namespace", namespace, "--include-crds",
    ]
    for item in sets or []:
        cmd.extend(["--set", item])
    return cmd


def helm_template(namespace, sets=None):
    docs = load_docs(run(helm_command(namespace, [f"zone={ZONE}"] + (sets or [])), echo_stdout=False))
    crds = {doc["metadata"]["name"] for doc in docs if doc.get("kind") == "CustomResourceDefinition"}
    expected = {
        "volumesnapshotclasses.snapshot.storage.k8s.io",
        "volumesnapshotcontents.snapshot.storage.k8s.io",
        "volumesnapshots.snapshot.storage.k8s.io",
    }
    if crds != expected:
        raise SystemExit(f"unexpected bundled CRDs: {crds!r}")
    return [doc for doc in docs if doc.get("kind") != "CustomResourceDefinition"]


def check_render_failure(sets, message):
    cmd = helm_command("kube-system", sets)
    print("+", " ".join(cmd), "(expect failure)", flush=True)
    result = subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True)
    if result.returncode == 0 or message not in result.stderr:
        print(f"expected rendering to fail with {message!r}: {result.stderr}")
        return False
    return True


def check_default_class(docs, expected):
    defaults = {
        doc["metadata"]["name"]
        for doc in docs if doc.get("kind") == "StorageClass"
        and doc["metadata"].get("annotations", {}).get(
            "storageclass.kubernetes.io/is-default-class"
        ) == "true"
    }
    wanted = {expected} if expected else set()
    if defaults != wanted:
        print(f"incorrect default StorageClasses: expected {wanted!r}, got {defaults!r}")
        return False
    return True


def find(docs, kind, name):
    for doc in docs:
        if doc.get("kind") == kind and doc["metadata"]["name"] == name:
            return doc
    raise SystemExit(f"rendered chart is missing {kind}/{name}")


def set_expected_chart_workload_changes(docs):
    binding = find(docs, "RoleBinding", "snapshot-controller-leaderelection-binding")
    binding["subjects"] = [{
        "kind": "ServiceAccount",
        "name": "csi-upcloud-controller-sa",
        "namespace": "kube-system",
    }]
    node = find(docs, "DaemonSet", "csi-upcloud-node")
    node_plugin = node["spec"]["template"]["spec"]["containers"][-1]
    node_plugin["args"] = [
        "--endpoint=$(CSI_ENDPOINT)",
        "--nodehost=$(NODE_ID)",
        "--mode=node",
        f"--zone={ZONE}",
    ]
    node_plugin["env"] = [
        env for env in node_plugin["env"]
        if env["name"] not in {"UPCLOUD_USERNAME", "UPCLOUD_PASSWORD"}
    ]

    controller = find(docs, "Deployment", "csi-upcloud-controller")
    # Explicit null also clears the API server's default RollingUpdate on upgrade.
    controller["spec"]["strategy"] = {"type": "Recreate", "rollingUpdate": None}
    controller_plugin = controller["spec"]["template"]["spec"]["containers"][-1]
    controller_plugin["args"] = [
        "--endpoint=$(CSI_ENDPOINT)",
        "--mode=controller",
        f"--zone={ZONE}",
    ]
    controller_plugin["env"] = [
        env for env in controller_plugin["env"] if env["name"] != "NODE_ID"
    ]
    for env in controller_plugin["env"]:
        if env["name"] in {"UPCLOUD_USERNAME", "UPCLOUD_PASSWORD"}:
            env["valueFrom"]["secretKeyRef"]["name"] = "upcloud-csi-credentials"
            env["valueFrom"]["secretKeyRef"]["optional"] = True
    controller_plugin["env"].append(
        {
            "name": "UPCLOUD_TOKEN",
            "valueFrom": {
                "secretKeyRef": {
                    "name": "upcloud-csi-credentials",
                    "key": "token",
                    "optional": True,
                }
            },
        }
    )

    for doc in docs:
        if doc.get("kind") not in {"Deployment", "DaemonSet"}:
            continue
        for container in doc["spec"]["template"]["spec"]["containers"]:
            container["image"] = container["image"].replace("k8s.gcr.io/", "registry.k8s.io/")
            if container["name"] == "csi-upcloud-plugin":
                container["image"] = "ghcr.io/upcloudltd/upcloud-csi:v1.5.1"


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


def check_credentials(plugin, secret_name):
    refs = {
        env["name"]: env["valueFrom"]["secretKeyRef"]
        for env in plugin["env"]
        if env["name"].startswith("UPCLOUD_")
    }
    expected = {
        f"UPCLOUD_{key.upper()}": {"name": secret_name, "key": key, "optional": True}
        for key in ("username", "password", "token")
    }
    if refs != expected:
        print(f"incorrect credential Secret references: {refs!r}")
        return False

    # Model Kubernetes' optional key resolution for each supported Secret shape.
    for secret in (
        {"username": "user", "password": "password"},
        {"token": "token"},
        {"username": "user", "password": "password", "token": "token"},
    ):
        resolved = {name: secret.get(ref["key"], "") for name, ref in refs.items()}
        if not (resolved["UPCLOUD_TOKEN"] or (
            resolved["UPCLOUD_USERNAME"] and resolved["UPCLOUD_PASSWORD"]
        )):
            print(f"credentials could not be resolved from Secret keys {sorted(secret)}")
            return False
    return True


def main():
    run(["helm", "lint", str(CHART), "--set", f"zone={ZONE}"])
    ok = check_render_failure([], "zone is required:")
    ok = check_render_failure(["zone="], "zone is required:") and ok
    ok = check_render_failure(["zone=   "], "zone is required:") and ok
    ok = check_render_failure(["zone=false"], "zone must be a string") and ok
    ok = check_render_failure(
        [f"zone={ZONE}", "storageClasses.defaultClass=unknown"],
        "is not a chart-managed StorageClass",
    ) and ok
    ok = check_render_failure(
        [f"zone={ZONE}", "storageClasses.defaultClass=false"],
        "storageClasses.defaultClass must be a StorageClass name or an empty string",
    ) and ok
    rendered = helm_template("kube-system")
    manifest = []
    for path in STATIC:
        manifest.extend(load_file(path))
    set_expected_chart_workload_changes(manifest)
    print(f"comparing {len(rendered)} rendered resources with the static manifests")
    for doc in rendered:
        metadata = doc.get("metadata") or {}
        if doc.get("kind") in CLUSTER_SCOPED and "namespace" in metadata:
            print(f"{doc['kind']}/{metadata.get('name')} sets metadata.namespace")
            ok = False
    if not SHARED_SNAPSHOT_RESOURCES <= set(index(rendered)):
        print("shared snapshot controller resources must be present by default")
        ok = False
    ok = compare(rendered, manifest, skip={ENCRYPTED_CLASS}) and ok

    snapshot_class = find(rendered, "VolumeSnapshotClass", "upcloud-csi-snapshotclass")
    if (snapshot_class.get("apiVersion"), snapshot_class.get("driver"),
        snapshot_class.get("deletionPolicy")) != (
        "snapshot.storage.k8s.io/v1", "storage.csi.upcloud.com", "Delete"
    ):
        print("incorrect VolumeSnapshotClass")
        ok = False

    default_controller = find(rendered, "Deployment", "csi-upcloud-controller")
    default_node = find(rendered, "DaemonSet", "csi-upcloud-node")
    default_plugin = default_controller["spec"]["template"]["spec"]["containers"][-1]
    default_class = find(rendered, "StorageClass", "upcloud-block-storage-maxiops")
    if default_controller["spec"]["replicas"] != 1 or default_controller["spec"]["strategy"] != {
        "type": "Recreate", "rollingUpdate": None
    }:
        print("default controller must have one replica and use Recreate")
        ok = False
    ok = check_default_class(rendered, "upcloud-block-storage-maxiops") and ok
    encrypted_class = find(rendered, *ENCRYPTED_CLASS)
    expected_encrypted = normalize(default_class)
    expected_encrypted["metadata"]["name"] = ENCRYPTED_CLASS[1]
    expected_encrypted["metadata"]["annotations"]["storageclass.kubernetes.io/is-default-class"] = "false"
    expected_encrypted["parameters"]["encryption"] = "data-at-rest"
    if normalize(encrypted_class) != expected_encrypted:
        print("encrypted StorageClass must match maxiops with data-at-rest encryption and a non-default annotation")
        ok = False
    for workload in (default_controller, default_node):
        plugin = next(
            c for c in workload["spec"]["template"]["spec"]["containers"]
            if c["name"] == "csi-upcloud-plugin"
        )
        if [arg for arg in plugin["args"] if arg.startswith("--zone=")] != [f"--zone={ZONE}"]:
            print(f"{workload['kind']} has an incorrect zone")
            ok = False
    ok = check_credentials(default_plugin, "upcloud-csi-credentials") and ok

    pod = default_controller["spec"]["template"]["spec"]
    snapshotter = next(c for c in pod["containers"] if c["name"] == "csi-snapshotter")
    plugin = next(c for c in pod["containers"] if c["name"] == "csi-upcloud-plugin")
    if not (
        snapshotter["image"] == "registry.k8s.io/sig-storage/csi-snapshotter:v4.2.1"
        and "--csi-address=$(ADDRESS)" in snapshotter["args"]
        and "--leader-election=false" in snapshotter["args"]
        and {"name": "ADDRESS", "value": "/var/lib/csi/sockets/pluginproxy/csi.sock"} in snapshotter["env"]
        and {"name": "socket-dir", "mountPath": "/var/lib/csi/sockets/pluginproxy/"} in snapshotter["volumeMounts"]
        and {"name": "socket-dir", "mountPath": "/var/lib/csi/sockets/pluginproxy/"} in plugin["volumeMounts"]
        and {"name": "CSI_ENDPOINT", "value": "unix:///var/lib/csi/sockets/pluginproxy/csi.sock"} in plugin["env"]
        and pod["serviceAccount"] == "csi-upcloud-controller-sa"
    ):
        print("CSI snapshotter socket, image, or service account is incorrect")
        ok = False
    binding = find(rendered, "ClusterRoleBinding", "csi-upcloud-provisioner-binding")
    role = find(rendered, "ClusterRole", binding["roleRef"]["name"])
    if {"kind": "ServiceAccount", "name": pod["serviceAccount"], "namespace": "kube-system"} not in binding["subjects"]:
        print("CSI snapshotter service account is not bound to the provisioner role")
        ok = False
    for resource, verbs in {
        "volumesnapshotclasses": {"get", "list", "watch"},
        "volumesnapshotcontents": {"create", "get", "list", "watch", "update", "delete"},
        "volumesnapshotcontents/status": {"update"},
    }.items():
        if not any(
            "snapshot.storage.k8s.io" in rule["apiGroups"]
            and resource in rule["resources"]
            and verbs <= set(rule["verbs"])
            for rule in role["rules"]
        ):
            print(f"CSI snapshotter lacks required permissions for {resource}")
            ok = False

    for workload in (default_controller, default_node):
        for container in workload["spec"]["template"]["spec"]["containers"]:
            image = container["image"]
            if container["name"] == "csi-upcloud-plugin":
                valid = image == "ghcr.io/upcloudltd/upcloud-csi:v1.5.1"
            else:
                valid = bool(re.fullmatch(
                    r"registry\.k8s\.io/sig-storage/[\w-]+:v\d+\.\d+\.\d+", image
                ))
            if not valid:
                print(f"unexpected default image for {container['name']}: {image}")
                ok = False

    for selected in ("", "upcloud-block-storage-hdd", ENCRYPTED_CLASS[1]):
        classes = helm_template("kube-system", [f"storageClasses.defaultClass={selected}"])
        ok = check_default_class(classes, selected) and ok

    without_classes = helm_template("kube-system", [
        "storageClasses.enabled=false", "storageClasses.defaultClass=unknown",
    ])
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
            skip={ENCRYPTED_CLASS, ("VolumeSnapshotClass", "upcloud-csi-snapshotclass")},
        ) and ok

    without_shared = helm_template("kube-system", ["snapshotController.enabled=false"])
    if SHARED_SNAPSHOT_RESOURCES & set(index(without_shared)):
        print("snapshotController.enabled=false still rendered shared snapshot controller resources")
        ok = False
    ok = compare(without_shared, manifest, skip=SHARED_SNAPSHOT_RESOURCES | {ENCRYPTED_CLASS}) and ok
    shared = helm_template("kube-system", ["snapshotController.enabled=true"])
    ok = compare(shared, manifest, skip={ENCRYPTED_CLASS}) and ok
    shared_without_class = helm_template("kube-system", [
        "snapshotController.enabled=true", "volumeSnapshotClass.enabled=false",
    ])
    if set(index(shared)) - set(index(shared_without_class)) != {
        ("VolumeSnapshotClass", "upcloud-csi-snapshotclass")
    } or set(index(shared_without_class)) - set(index(shared)):
        print("volumeSnapshotClass.enabled must affect only the VolumeSnapshotClass")
        ok = False
    ok = compare(
        shared_without_class, manifest,
        skip={ENCRYPTED_CLASS, ("VolumeSnapshotClass", "upcloud-csi-snapshotclass")},
    ) and ok

    custom = helm_template(
        "csi-test",
        [
            "controller.replicas=3",
            "snapshotController.enabled=true",
            "snapshotController.replicas=4",
            "credentials.secretName=my-secret",
            "zone=de-fra1",
            "image.repository=example.com/csi",
            "image.tag=v9.9.9",
            "sidecars.provisioner.tag=v9.9.9",
            "sidecars.provisioner.repository=example.com/provisioner",
            "storageClasses.defaultClass=",
            "resources.node.plugin.requests.cpu=100m",
            "resources.node.plugin.requests.memory=128Mi",
            "resources.node.plugin.limits.memory=256Mi",
            "resources.controller.provisioner.requests.cpu=50m",
            "controller.tolerations[0].key=node-role.kubernetes.io/control-plane",
            "controller.tolerations[0].operator=Exists",
            "controller.tolerations[0].effect=NoSchedule",
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
        find(custom, "RoleBinding", "snapshot-controller-leaderelection-binding")["subjects"] == [{
            "kind": "ServiceAccount",
            "name": "csi-upcloud-controller-sa",
            "namespace": "csi-test",
        }],
        "namespace" not in driver.get("metadata", {}),
        "namespace" not in storage_class["metadata"],
        "namespace" not in find(custom, "ClusterRole", "csi-upcloud-node-driver-registrar-role")["metadata"],
        "namespace" not in find(custom, "VolumeSnapshotClass", "upcloud-csi-snapshotclass")["metadata"],
        binding["subjects"][0]["namespace"] == "csi-test",
        storage_class["metadata"]["annotations"]["storageclass.kubernetes.io/is-default-class"] == "false",
        controller["spec"]["strategy"] == {"type": "Recreate", "rollingUpdate": None},
    ]
    plugin = controller["spec"]["template"]["spec"]["containers"][-1]
    provisioner = controller["spec"]["template"]["spec"]["containers"][0]
    node_plugin = node["spec"]["template"]["spec"]["containers"][-1]
    controller_pod_spec = controller["spec"]["template"]["spec"]
    node_pod_spec = node["spec"]["template"]["spec"]
    controller_plugin_env = {env["name"]: env for env in plugin["env"]}
    node_plugin_env = {env["name"]: env for env in node_plugin["env"]}
    default_controller_pod_spec = find(
        rendered, "Deployment", "csi-upcloud-controller"
    )["spec"]["template"]["spec"]
    checks.extend(
        [
            plugin["image"] == "example.com/csi:v9.9.9",
            node_plugin["image"] == "example.com/csi:v9.9.9",
            provisioner["image"] == "example.com/provisioner:v9.9.9",
            "--mode=controller" in plugin["args"],
            "--zone=de-fra1" in plugin["args"],
            "NODE_ID" not in controller_plugin_env,
            controller_plugin_env["UPCLOUD_USERNAME"]["valueFrom"]["secretKeyRef"]["name"] == "my-secret",
            controller_plugin_env["UPCLOUD_PASSWORD"]["valueFrom"]["secretKeyRef"]["name"] == "my-secret",
            "--mode=node" in node_plugin["args"],
            "--zone=de-fra1" in node_plugin["args"],
            "--nodehost=$(NODE_ID)" in node_plugin["args"],
            "NODE_ID" in node_plugin_env,
            node_plugin_env["NODE_ID"]["valueFrom"]["fieldRef"]["fieldPath"] == "spec.nodeName",
            "UPCLOUD_USERNAME" not in node_plugin_env,
            "UPCLOUD_PASSWORD" not in node_plugin_env,
            "UPCLOUD_TOKEN" not in node_plugin_env,
            not any(
                env["name"].startswith("UPCLOUD_")
                for container in node_pod_spec["containers"]
                for env in container.get("env", [])
            ),
            not any(
                "secretRef" in source
                for container in node_pod_spec["containers"]
                for source in container.get("envFrom", [])
            ),
            not any(
                "secretKeyRef" in env.get("valueFrom", {})
                for container in node_pod_spec["containers"]
                for env in container.get("env", [])
            ),
            not any(
                "secret" in volume
                for volume in node_pod_spec.get("volumes", [])
            ),
            "tolerations" not in default_controller_pod_spec,
            controller_pod_spec["tolerations"] == [
                {
                    "key": "node-role.kubernetes.io/control-plane",
                    "operator": "Exists",
                    "effect": "NoSchedule",
                }
            ],
            "tolerations" not in node_pod_spec,
            node_plugin["resources"]["requests"]["cpu"] == "100m",
            node_plugin["resources"]["requests"]["memory"] == "128Mi",
            node_plugin["resources"]["limits"]["memory"] == "256Mi",
            provisioner["resources"]["requests"]["cpu"] == "50m",
            "resources" not in controller["spec"]["template"]["spec"]["containers"][1],
        ]
    )
    ok = check_credentials(plugin, "my-secret") and ok
    if not all(checks):
        print("custom values were not applied as expected")
        print("plugin", plugin["image"], plugin["env"])
        print("provisioner", provisioner["image"], provisioner.get("resources"))
        print("node", node_plugin["image"], node_plugin.get("resources"))
        print("controller replicas", controller["spec"]["replicas"], controller["metadata"])
        ok = False

    if not ok:
        sys.exit(1)
    print("helm chart matches the static manifests with expected chart differences")


if __name__ == "__main__":
    main()
