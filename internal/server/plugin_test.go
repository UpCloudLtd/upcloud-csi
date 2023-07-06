package server_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/identity"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/server"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestPluginServer(t *testing.T) {
	t.Parallel()

	l := logger.New("info").WithField("package", "server_test")
	tmpSocket := path.Join(os.TempDir(), fmt.Sprintf("test-plugin-server-%d.sock", time.Now().Unix()))
	addr := fmt.Sprintf("unix://%s", tmpSocket)
	defer func() {
		os.Remove(tmpSocket)
	}()

	srv, err := server.NewPluginServer(addr, nil, nil, identity.NewIdentity(config.DefaultDriverName, l), l)
	require.NoError(t, err)

	go func() {
		t.Logf("starting GRPC Plugin server at %s", addr)
		if err := srv.Run(); err != nil {
			t.Log(err)
		}
	}()
	time.Sleep(2 * time.Second)
	require.FileExists(t, tmpSocket)

	t.Logf("connection GRPC server at %s", addr)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	id := csi.NewIdentityClient(conn)

	t.Log("get plugin info to make sure that server is responding")
	info, err := id.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	require.NoError(t, err)
	require.Equal(t, config.DefaultDriverName, info.Name)

	srv.Stop(nil)
	t.Logf("check that socket %s is removed", tmpSocket)
	require.NoFileExists(t, tmpSocket)
}
