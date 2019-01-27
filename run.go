package main

import (
        "context"
	"encoding/json"
	"strings"
	"os"
	"fmt"
	"flag"
	"os/exec"
	"bufio"

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

const runHelp = `Extract a rootfs from a Docker image and run it in a VM`
func (cmd *runCommand) Name() string { return "run" }
func (cmd *runCommand) Args() string { return "[options] docker.io/organization/name:tag" }
func (cmd *runCommand) ShortHelp() string { return runHelp }
func (cmd *runCommand) LongHelp() string { return runHelp }
func (cmd *runCommand) Hidden() bool { return false }

func (cmd *runCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&cmd.namespace, "namespace", "docker-to-firecracker", "ContainerD namespace to fetch images [default docker-to-firecracker]")
	fs.StringVar(&cmd.containerdSock, "containerdsock", "/run/containerd/containerd.sock", "Path to ContainerD socket [default /run/containerd/containerd.sock]")
	fs.StringVar(&cmd.tmpMountPoint, "tmp-mnt", "/mnt", "Path to temporarily mount on the host file system to generate the root filesystem [default /mnt]")
	fs.StringVar(&cmd.rootFSPath, "rootfs-path", "./disk.img", "Path to generate a root filesystem [default ./disk.image]")
	fs.BoolVar(&cmd.generateBootInit, "generate-boot-init", true, "Generate boot init script and write to root filesystem [default true]")
	fs.StringVar(&cmd.bootInitFileName, "boot-init-file-name", "custom.init", "Set the path to the generated boot init script [default custom.init]")
	fs.StringVar(&cmd.bootInitArg, "boot-init-arg", "/custom.init", "Set the boot init arg to pass to Firecracker VM [default /custom.init]")
	fs.StringVar(&cmd.kernelPath, "kernel-path", "./hello-vmlinux.bin", "Path the Kernel to pass to Firecracker VM [default ./hello-vmlinux.bin]")
	fs.StringVar(&cmd.firecrackerPath, "firecracker-path", "./firecracker", "Path to the Firecracker VM binary")
	fs.StringVar(&cmd.firecrackerSock, "firecracker-sock", "./firecracker.sock", "Path to a temporary Firecracker socket")
}

type runCommand struct {
	namespace string
	containerdSock string
	tmpMountPoint string
	rootFSPath string
	generateBootInit bool
	bootInitFileName string
	bootInitArg string
	kernelPath string
	firecrackerPath string
	firecrackerSock string
}

func (cmd *runCommand) Run(ctx context.Context, args []string) (err error) {
	if len(args) < 1 {
		return fmt.Errorf("Must pass a docker url: docker.io/organization/name:tag")
	}
	logger := log.New()
	imageName := args[0]

	client, err := containerd.New(cmd.containerdSock)
        if err != nil {
		return err
        }
        defer client.Close()
        log.Printf("Pulling Image: %s\n", imageName)
        ctx = namespaces.WithNamespace(ctx, cmd.namespace)
        // pull an image
        img, err := client.Fetch(ctx, imageName)
        if err != nil {
		return err
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
	initEnvs := imageSpec.Config.Env

	log.Printf("Creating Root FS\n")
	command := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", cmd.rootFSPath), "bs=1", "count=0", "seek=1G")
	if err := command.Run(); err != nil {
		return err
	}


	command = exec.Command("mkfs.ext4", "-F", cmd.rootFSPath)
	if err := command.Run(); err != nil {
		return err
	}

	command = exec.Command("mount", "-o", "loop", cmd.rootFSPath, cmd.tmpMountPoint)
	if err := command.Run(); err != nil {
		return err
	}

	// unpack the image to a root fs
	log.Printf("Upacking Image: %s", imageName)
	manifest, err := images.Manifest(ctx, client.ContentStore(), img.Target, platforms.Default())
        if err != nil {
		return err
        }
	for _, desc := range manifest.Layers {
		log.Printf("Upacking Layer: %s", desc.Digest.String())
		layer, err := client.ContentStore().ReaderAt(ctx, desc)
		if err != nil {
			return err
		}
		if err := archive.Untar(content.NewReader(layer), cmd.tmpMountPoint, &archive.TarOptions{
			NoLchown: true,
		}); err != nil {
			return err
		}
	}

	if (cmd.generateBootInit) {
		log.Printf("Generating Boot Init Script")
		// create init script -- this will write over /sbin/init if it already exists
		initScriptLocaton := fmt.Sprintf("%s/%s", cmd.tmpMountPoint, cmd.bootInitFileName)
		// TODO: handle deeper paths for init script
		f, err := os.Create(initScriptLocaton)
		if err != nil {
			return err
		}
		writer := bufio.NewWriter(f)
		fmt.Fprintf(writer, "#!/bin/sh\n")
		for _, env := range initEnvs {
			fmt.Fprintf(writer, "export %s\n", env)
		}
		fmt.Fprintf(writer, "%s\n", initCmd)
		writer.Flush()
		f.Sync()
		f.Close()
		mode := int(0755)
		os.Chmod(initScriptLocaton, os.FileMode(mode))
	}

	command = exec.Command("umount", cmd.tmpMountPoint)
	if err := command.Run(); err != nil {
		return err
	}
	vmmCtx, vmmCancel := context.WithCancel(ctx)
        defer vmmCancel()
        devices := []models.Drive{}
        rootDrive := models.Drive{
                DriveID:      firecracker.String("1"),
                PathOnHost:   &cmd.rootFSPath,
                IsRootDevice: firecracker.Bool(true),
                IsReadOnly:   firecracker.Bool(false),
        }
        devices = append(devices, rootDrive)
        fcCfg := firecracker.Config{
                SocketPath:        cmd.firecrackerSock,
                KernelImagePath:   cmd.kernelPath,
		KernelArgs:        fmt.Sprintf("console=ttyS0 reboot=k panic=1 pci=off init=\"%s\"", cmd.bootInitArg),
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
        command = firecracker.VMCommandBuilder{}.
                WithBin(cmd.firecrackerPath).
                WithSocketPath(fcCfg.SocketPath).
                WithStdin(os.Stdin).
                WithStdout(os.Stdout).
                WithStderr(os.Stderr).
                Build(ctx)
        machineOpts = append(machineOpts, firecracker.WithProcessRunner(command))
        m, err := firecracker.NewMachine(vmmCtx, fcCfg, machineOpts...)
        if err != nil {
		return err
        }

        if err := m.Start(vmmCtx); err != nil {
		return err
        }
        defer m.StopVMM()

        // wait for the VMM to exit
        if err := m.Wait(vmmCtx); err != nil {
		return err
        }
        log.Printf("Start machine was happy")
	return nil
}
