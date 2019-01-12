package main
import (
        "bytes"
	"context"
        "log"
        "encoding/json"

        "github.com/containerd/containerd"
        "github.com/containerd/containerd/namespaces"
        "github.com/containerd/containerd/content"
        //ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func main() {
        client, err := containerd.New("/run/containerd/containerd.sock")
        if err != nil {
                return
        }
        defer client.Close()

	ctx := namespaces.WithNamespace(context.Background(), "example")
	// pull an image
        image, err := client.Pull(ctx, "docker.io/library/hello-world:latest", containerd.WithPullUnpack)
        log.Printf("Pulled Image: %s\n", image.Name())

	// extract the image spec
	config, err := image.Config(ctx)
        provider := client.ContentStore()
        imageSpec, err := content.ReadBlob(ctx, provider, config)
	// NOTE: use ocispec to parse the imageSpec, keeping this around
	// 	 since this just outputs the json
        //var imageSpec ocispec.Image
        //json.Unmarshal(p, &imageSpec)
	var prettyImageSpec bytes.Buffer
	json.Indent(&prettyImageSpec, imageSpec, "", "\t")
        log.Printf("Config: %s\n", string(prettyImageSpec.Bytes()))
}
