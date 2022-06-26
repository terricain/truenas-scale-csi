package driver

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	tnclient "github.com/terrycain/truenas-go-sdk"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	mount "k8s.io/mount-utils"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

const (
	NFSDriverName = "nfs.truenas.terrycain.github.com"
	ISCSIDriverName = "iscsi.truenas.terrycain.github.com"
)

var (
	GitTreeState = "not a git tree"
	Commit       string
	Version      string
)

type Driver struct {
	name string
	baseURL string
	address string

	nfsStoragePath string
	nodeID        string
	client        *tnclient.APIClient
	isController  bool
	isNFS bool

	srv *grpc.Server
	endpoint string
	mounter mount.Interface

	readyMu sync.Mutex // protects ready
	ready   bool
}

func NewDriver(endpoint, baseUrl, accessToken, nfsStoragePath string, isController bool, nodeID string, isNFS bool) (*Driver, error) {
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to parse address: %w", err)
	}
	if !strings.HasSuffix(u.Path, "api/v2.0") {
		return nil, fmt.Errorf("base URL should end with \"api/v2.0\": %s", u.Path)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(context.Background(), ts)
	config := tnclient.NewConfiguration()
	config.Servers = tnclient.ServerConfigurations{{URL: baseUrl}}
	config.Debug = true
	config.HTTPClient = tc
	client := tnclient.NewAPIClient(config)

	driverName := NFSDriverName
	if !isNFS {
		driverName = ISCSIDriverName
	}

	return &Driver{
		name:          driverName,
		baseURL: baseUrl,
		address: u.Host,
		nfsStoragePath:           nfsStoragePath,
		nodeID:        nodeID,
		client: client,
		isController:  isController,
		isNFS: isNFS,
		endpoint: endpoint,
		mounter: mount.New(""),
	}, nil
}

func (d *Driver) Run(ctx context.Context) error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %w", err)
	}

	grpcAddr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		grpcAddr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	}

	// Remove socket if it exists
	if err := os.Remove(grpcAddr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old unix domain socket file %s, error: %w", grpcAddr, err)
	}

	sockPath := path.Dir(u.Path)
	if err := os.MkdirAll(sockPath, 0o750); err != nil {
		return fmt.Errorf("failed to make directories for sock, error: %w", err)
	}

	// TODO(iscsi)
	//d.configDir = path.Join(sockPath, "config")
	//if err = os.MkdirAll(d.configDir, 0o750); err != nil {
	//	return fmt.Errorf("failed to make directories for config, error: %w", err)
	//}

	grpcListener, err := net.Listen(u.Scheme, grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			log.Error().Err(err).Str("method", info.FullMethod).Msg("method failed")
		}
		return resp, err
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	d.setReady(true)
	log.Info().Str("grpc_addr", grpcAddr).Msg("starting CSI GRPC server")

	var eg errgroup.Group
	eg.Go(func() error {
		go func() {
			<-ctx.Done()
			log.Info().Msg("Server stopped")
			d.setReady(false)
			d.srv.GracefulStop()
		}()
		return d.srv.Serve(grpcListener)
	})

	return eg.Wait()
}

func (d *Driver) setReady(state bool) {
	d.readyMu.Lock()
	defer d.readyMu.Unlock()
	d.ready = state
}