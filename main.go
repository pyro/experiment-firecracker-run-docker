
package main
import (
        "context"
	"encoding/json"
	"strings"
	"os"
	"fmt"
	"os/exec"

        "github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/content"
        "github.com/containerd/containerd/namespaces"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/docker/docker/pkg/archive"
	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	log "github.com/sirupsen/logrus"
)

func main() {
	logger := log.New()
	dest := "/mnt"
	imageName := "docker.io/library/hello-world:latest"
        client, err := containerd.New("/run/containerd/containerd.sock")
        if err != nil {
		log.Fatal(err)
        }
        defer client.Close()
        log.Printf("Pulling Image: %s\n", imageName)
        ctx := namespaces.WithNamespace(context.Background(), "example")
        // pull an image
        img, err := client.Fetch(ctx, imageName)
        if err != nil {
		log.Fatal(err)
        }

	// extract the init command
	log.Printf("Extracting init CMD\n")
	provider := client.ContentStore()
	platform := platforms.Default()
	config, err := img.Config(ctx, provider, platform)
	configBlob, err := content.ReadBlob(ctx, provider, config)
	var imageSpec ocispec.Image
	json.Unmarshal(configBlob, &imageSpec)
	initCmd := strings.Join(imageSpec.Config.Cmd, " ")

	log.Printf("Creating Root FS\n")
	cmd := exec.Command("dd", "if=/dev/zero", "of=disk.img", "bs=1", "count=0", "seek=1G")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}


	cmd = exec.Command("mkfs.ext4", "-F", "disk.img")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("mount", "-o", "loop", "disk.img", "/mnt")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	// unpack the image to a root fs
	log.Printf("Upacking Image: %s", imageName)
	manifest, err := images.Manifest(ctx, client.ContentStore(), img.Target, platforms.Default())
        if err != nil {
		log.Fatal(err)
        }
	for _, desc := range manifest.Layers {
		log.Printf("Upacking Layer: %s", desc.Digest.String())
		layer, err := client.ContentStore().ReaderAt(ctx, desc)
		if err != nil {
			log.Fatal("Error Finding Layer With Digest: %s", desc.Digest.String())
		}
		if err := archive.Untar(content.NewReader(layer), dest, &archive.TarOptions{
			NoLchown: true,
		}); err != nil {
			log.Fatal("Error extracting tar for layer %s to directory %s", desc.Digest.String(), dest)
		}
	}

	cmd = exec.Command("umount", "/mnt")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
	vmmCtx, vmmCancel := context.WithCancel(ctx)
        defer vmmCancel()
        devices := []models.Drive{}
        rootDrivePath := "./disk.img"
        rootDrive := models.Drive{
                DriveID:      firecracker.String("1"),
                PathOnHost:   &rootDrivePath,
                IsRootDevice: firecracker.Bool(true),
                IsReadOnly:   firecracker.Bool(false),
        }
        devices = append(devices, rootDrive)
        kernelImagePath := "./hello-vmlinux.bin"
        fcCfg := firecracker.Config{
                SocketPath:        "./firecracker.sock",
                KernelImagePath:   kernelImagePath,
		KernelArgs:        fmt.Sprintf("console=ttyS0 reboot=k panic=1 pci=off init=\"%s\"", initCmd),
                Drives:            devices,
                MachineCfg: models.MachineConfiguration{
                        VcpuCount:   1,
                        CPUTemplate: models.CPUTemplate("C3"),
                        HtEnabled:   true,
                        MemSizeMib:  512,
                },
        }
        machineOpts := []firecracker.Opt{
                firecracker.WithLogger(log.NewEntry(logger)),
        }
        cmd = firecracker.VMCommandBuilder{}.
                WithBin("./firecracker-v0.10.1").
                WithSocketPath(fcCfg.SocketPath).
                WithStdin(os.Stdin).
                WithStdout(os.Stdout).
                WithStderr(os.Stderr).
                Build(ctx)
        machineOpts = append(machineOpts, firecracker.WithProcessRunner(cmd))
        m, err := firecracker.NewMachine(vmmCtx, fcCfg, machineOpts...)
        if err != nil {
                log.Fatalf("Failed creating machine: %s", err)
        }
        if err := m.Start(vmmCtx); err != nil {
                log.Fatalf("Failed to start machine: %v", err)
        }
        defer m.StopVMM()
        // wait for the VMM to exit
        if err := m.Wait(vmmCtx); err != nil {
                log.Fatalf("Wait returned an error %s", err)
        }
        log.Printf("Start machine was happy")
}
