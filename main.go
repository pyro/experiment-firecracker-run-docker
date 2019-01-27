package main
import (
	"github.com/genuinetools/pkg/cli"
)

func main() {
	p := cli.NewProgram()
	p.Name = "docker-to-firecracker"
	p.Description = "Extract a rootfs from a Docker image and run it in a VM"
	p.Commands = []cli.Command{
		&runCommand{},
	}
	p.Run()
}
