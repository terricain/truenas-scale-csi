package driver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tnclient "github.com/terrycain/truenas-go-sdk"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	mount "k8s.io/mount-utils"
)

const (
	NFSDriverName   = "nfs.truenas-scale.terrycain.github.com"
	ISCSIDriverName = "iscsi.truenas-scale.terrycain.github.com"
)

var (
	driverVersion string
	gitCommit     string
	buildDate     string
)

type VersionInfo struct {
	DriverVersion string `json:"driverVersion"`
	GitCommit     string `json:"gitCommit"`
	BuildDate     string `json:"buildDate"`
	GoVersion     string `json:"goVersion"`
	Compiler      string `json:"compiler"`
	Platform      string `json:"platform"`
}

func GetVersion() VersionInfo {
	return VersionInfo{
		DriverVersion: driverVersion,
		GitCommit:     gitCommit,
		BuildDate:     buildDate,
		GoVersion:     runtime.Version(),
		Compiler:      runtime.Compiler,
		Platform:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func GetVersionJSON() (string, error) {
	info := GetVersion()
	marshalled, err := json.MarshalIndent(&info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(marshalled), nil
}

type Driver struct {
	name    string
	baseURL string
	address string

	nfsStoragePath   string
	iscsiStoragePath string
	nodeID           string
	client           *tnclient.APIClient
	isController     bool
	isNFS            bool
	portalID         int32
	iscsiConfigDir   string

	srv      *grpc.Server
	endpoint string
	mounter  mount.Interface

	readyMu sync.Mutex // protects ready
	ready   bool
}

func NewDriver(endpoint, baseURL, accessToken, nfsStoragePath, iscsiStoragePath string, portalID int32, isController bool, nodeID string, isNFS, debugLogging bool, ignoreTLS bool) (*Driver, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse address: %w", err)
	}
	if !strings.HasSuffix(u.Path, "api/v2.0") {
		return nil, fmt.Errorf("base URL should end with \"api/v2.0\": %s", u.Path)
	}

	apiCtx := context.Background()
	tr := &http.Transport{
		// This defaults to false
		TLSClientConfig: &tls.Config{InsecureSkipVerify: ignoreTLS}, //nolint:gosec
	}
	tlsClient := &http.Client{Transport: tr}
	apiCtx = context.WithValue(apiCtx, oauth2.HTTPClient, tlsClient)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(apiCtx, ts)
	config := tnclient.NewConfiguration()
	config.Servers = tnclient.ServerConfigurations{tnclient.ServerConfiguration{URL: baseURL}}
	config.Debug = debugLogging
	config.HTTPClient = tc
	client := tnclient.NewAPIClient(config)

	driverName := NFSDriverName
	if !isNFS {
		driverName = ISCSIDriverName
	}

	return &Driver{
		name:             driverName,
		baseURL:          baseURL,
		address:          u.Host,
		nfsStoragePath:   nfsStoragePath,
		iscsiStoragePath: iscsiStoragePath,
		portalID:         portalID,
		nodeID:           nodeID,
		client:           client,
		isController:     isController,
		isNFS:            isNFS,
		endpoint:         endpoint,
		mounter:          mount.New(""),
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
	if err = os.Remove(grpcAddr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old unix domain socket file %s, error: %w", grpcAddr, err)
	}

	sockPath := path.Dir(u.Path)
	if err = os.MkdirAll(sockPath, 0o750); err != nil {
		return fmt.Errorf("failed to make directories for sock, error: %w", err)
	}

	if !d.isNFS {
		d.iscsiConfigDir = path.Join(sockPath, "iscsi_config")
		if err = os.MkdirAll(d.iscsiConfigDir, 0o750); err != nil {
			return fmt.Errorf("failed to make directories for config, error: %w", err)
		}
	}

	grpcListener, err := net.Listen(u.Scheme, grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			klog.ErrorS(err, "GRPC method failed", "method", info.FullMethod)
		}
		return resp, err
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	d.setReady(true)
	klog.V(4).InfoS("starting CSI GRPC server", "grpcAddress", grpcAddr)

	var eg errgroup.Group
	eg.Go(func() error {
		go func() {
			<-ctx.Done()
			klog.V(4).Info("GRPC server stopped")
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

func (d *Driver) getISCSILibConfigPath(id string) string {
	return path.Join(d.iscsiConfigDir, id+".json")
}
