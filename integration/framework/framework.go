package framework

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/cadvisor/client"
)

var host = flag.String("host", "localhost", "Address of the host being tested")
var port = flag.Int("port", 8080, "Port of the application on the host being tested")

// Integration test framework.
type Framework interface {
	// Clean the framework state.
	Cleanup()

	// The testing.T used by the framework and the current test.
	T() *testing.T

	// Returns information about the host being tested.
	Host() HostInfo

	// Returns the Docker actions for the test framework.
	Docker() DockerActions

	// Returns the cAdvisor actions for the test framework.
	Cadvisor() CadvisorActions
}

// Instantiates a Framework. Cleanup *must* be called. Class is thread-compatible.
// All framework actions report fatal errors on the t specified at creation time.
//
// Typical use:
//
// func TestFoo(t *testing.T) {
// 	fm := framework.New(t)
// 	defer fm.Cleanup()
//      ... actual test ...
// }
func New(t *testing.T) Framework {
	// All integration tests are large.
	if testing.Short() {
		t.Skip("Skipping framework test in short mode")
	}

	return &realFramework{
		host: HostInfo{
			Host: *host,
			Port: *port,
		},
		t:        t,
		cleanups: make([]func(), 0),
	}
}

type DockerActions interface {
	// Run the no-op pause Docker container and return its ID.
	RunPause() string

	// Run the specified command in a Docker busybox container and return its ID.
	RunBusybox(cmd ...string) string

	// Runs a Docker container in the background. Uses the specified DockerRunArgs and command.
	// Returns the ID of the new container.
	//
	// e.g.:
	// Run(DockerRunArgs{Image: "busybox"}, "ping", "www.google.com")
	//   -> docker run busybox ping www.google.com
	Run(args DockerRunArgs, cmd ...string) string
}

type CadvisorActions interface {
	// Returns a cAdvisor client to the machine being tested.
	Client() *client.Client
}

type realFramework struct {
	host           HostInfo
	t              *testing.T
	cadvisorClient *client.Client

	// Cleanup functions to call on Cleanup()
	cleanups []func()
}

type HostInfo struct {
	Host string
	Port int
}

// Returns: http://<host>:<port>/
func (self HostInfo) FullHost() string {
	return fmt.Sprintf("http://%s:%d/", self.Host, self.Port)
}

func (self *realFramework) T() *testing.T {
	return self.t
}

func (self *realFramework) Host() HostInfo {
	return self.host
}

func (self *realFramework) Docker() DockerActions {
	return self
}

func (self *realFramework) Cadvisor() CadvisorActions {
	return self
}

// Call all cleanup functions.
func (self *realFramework) Cleanup() {
	for _, cleanupFunc := range self.cleanups {
		cleanupFunc()
	}
}

// Gets a client to the cAdvisor being tested.
func (self *realFramework) Client() *client.Client {
	if self.cadvisorClient == nil {
		cadvisorClient, err := client.NewClient(self.Host().FullHost())
		if err != nil {
			self.t.Fatalf("Failed to instantiate the cAdvisor client: %v", err)
		}
		self.cadvisorClient = cadvisorClient
	}
	return self.cadvisorClient
}

func (self *realFramework) RunPause() string {
	return self.Run(DockerRunArgs{
		Image: "kubernetes/pause",
	}, "sleep", "inf")
}

// Run the specified command in a Docker busybox container.
func (self *realFramework) RunBusybox(cmd ...string) string {
	return self.Run(DockerRunArgs{
		Image: "busybox",
	}, cmd...)
}

type DockerRunArgs struct {
	// Image to use.
	Image string

	// Arguments to the Docker CLI.
	Args []string
}

// Runs a Docker container in the background. Uses the specified DockerRunArgs and command.
//
// e.g.:
// RunDockerContainer(DockerRunArgs{Image: "busybox"}, "ping", "www.google.com")
//   -> docker run busybox ping www.google.com
func (self *realFramework) Run(args DockerRunArgs, cmd ...string) string {
	if self.host.Host == "localhost" {
		// Just run locally.
		out, err := exec.Command("docker", append(append(append([]string{"run", "-d"}, args.Args...), args.Image), cmd...)...).CombinedOutput()
		if err != nil {
			self.t.Fatalf("Failed to run docker container with run args %+v due to error: %v and output: %q", args, err, out)
			return ""
		}
		// The last lime is the container ID.
		elements := strings.Split(string(out), "\n")
		if len(elements) < 2 {
			self.t.Fatalf("Failed to find Docker container ID in output %q", out)
			return ""
		}
		containerId := elements[len(elements)-2]
		self.cleanups = append(self.cleanups, func() {
			out, err := exec.Command("docker", "rm", "-f", containerId).CombinedOutput()
			if err != nil {
				glog.Errorf("Failed to remove container %q with error: %v and output: %q", containerId, err, out)
			}
		})
		return containerId
	}

	// TODO(vmarmol): Implement.
	// We must SSH to the remote machine and run the command.

	self.t.Fatalf("Non-localhost Run not implemented")
	return ""
}

// Runs retryFunc until no error is returned. After dur time the last error is returned.
// Note that the function does not timeout the execution of retryFunc when the limit is reached.
func RetryForDuration(retryFunc func() error, dur time.Duration) error {
	waitUntil := time.Now().Add(dur)
	var err error
	for time.Now().Before(waitUntil) {
		err = retryFunc()
		if err == nil {
			return nil
		}
	}
	return err
}
