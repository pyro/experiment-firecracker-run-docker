# [Experiment] Docker To Firecracker

Experimental CLI that takes a Docker image url and runs it in a Firecracker VM

Please do not use this in production for anything, _you're gonna have a bad time_.

## How Does This Work?

- Fetches an image using ContainerD
- Extracts CMD and ENV VARS from image metadata
- Creates an empty ext4 filesystem (mounts it at /mnt but default)
- Dumps the image rootfs in the the empty ext4 filesystem
- Creates an init script with CMD and ENV VARS image metatdata (unmounts /mnt by default)
- Starts a Firecracker VM with a Kernel + Docker Rootfs (Includes Docker ENV VARS + Docker CMD)

## Quick(ish) Start

1. Dev Environment

I've been using GCP VMs for this using these instuctions: https://github.com/firecracker-microvm/firecracker/blob/master/docs/dev-machine-setup.md#gcp

You _could_ also use an i3.metal intance on AWS but its an expensive instance (something like 72 cores and 512GB memory).

You could also run this on a a local Linux dev box if that's your thing.

2. Setup Go

If you're using Ubuntu (either 1.10 or 1.11 should work): https://github.com/golang/go/wiki/Ubuntu

If you're using something else I'll leave that up to you, just make sue you've got at least Golang 1.10

 (either 1.10 or 1.11 should work) (either 1.10 or 1.11 should work) (either 1.10 or 1.11 should work)

3. Install ContainerD

Follow the instructions here: https://containerd.io/docs/getting-started/#starting-containerd

You can also build ContainerD from source. ContainerD needs to be running for this to work.

4. Get Source and Build Binary

```
go get github.com/pyro/experiment-firecracker-run-docker-image
cd $GOPATH/src/github.com/pyro/experiment-firecracker-run-docker-image
make
sudo make install
```

_Make sure /usr/local/bin is in your PATH_

4. Download Firecracker Resources

```sh
# create a workspace (you can put this anywhere)
mkdir ~/DockerToFirecracker
cd ~/DockerToFirecracker
# Download Firecracker Binary
FC_VERSION=0.14.0
curl -LOJ https://github.com/firecracker-microvm/firecracker/releases/download/v${FC_VERSION}/firecracker-v${FC_VERSION}
mv firecracker-v${FC_VERSION} firecracker
chmod +x firecracker
# Download A Kernel Built For Firecracker
curl -fsSL -o hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin
```

5. Run A Docker Container On Firecracker

```sh
sudo docker-to-firecracker run docker.io/hharnisc/hello:latest

# you should see a bunch of output and eventually "hello, world!" (its hard to see in the logs)

[    0.880238] Write protecting the kernel read-only data: 12288k
[    0.908489] Freeing unused kernel memory: 2016K
[    0.922293] Freeing unused kernel memory: 584K
hello, world!
[    0.942806] Kernel panic - not syncing: Attempted to kill init! exitcode=0x00000000
[    0.942806]
```

Yes you should see a Kernel panic here because the init script is exiting. :D



You can also run Redis

```sh
sudo docker-to-firecracker run docker.io/library/redis:latest 

...

[    0.938889] Freeing unused kernel memory: 584K
[    0.976555] random: redis-server: uninitialized urandom read (4096 bytes read)
459:C 28 Jan 2019 16:20:02.571 # oO0OoO0OoO0Oo Redis is starting oO0OoO0OoO0Oo
459:C 28 Jan 2019 16:20:02.580 # Redis version=5.0.3, bits=64, commit=00000000, modified=0, pid=459, just started
459:C 28 Jan 2019 16:20:02.591 # Warning: no config file specified, using the default config. In order to specify a config file use redis-server /path/to/redis.conf
459:M 28 Jan 2019 16:20:02.608 * Increased maximum number of open files to 10032 (it was originally set to 1024).
                _._
           _.-``__ ''-._
      _.-``    `.  `_.  ''-._           Redis 5.0.3 (00000000/0) 64 bit
  .-`` .-```.  ```\/    _.,_ ''-._
 (    '      ,       .-`  | `,    )     Running in standalone mode
 |`-._`-...-` __...-.``-._|'` _.-'|     Port: 6379
 |    `-._   `._    /     _.-'    |     PID: 459
  `-._    `-._  `-./  _.-'    _.-'
 |`-._`-._    `-.__.-'    _.-'_.-'|
 |    `-._`-._        _.-'_.-'    |           http://redis.io
  `-._    `-._`-.__.-'_.-'    _.-'
 |`-._`-._    `-.__.-'    _.-'_.-'|
 |    `-._`-._        _.-'_.-'    |
  `-._    `-._`-.__.-'_.-'    _.-'
      `-._    `-.__.-'    _.-'
          `-._        _.-'
              `-.__.-'

459:M 28 Jan 2019 16:20:02.741 # Server initialized
459:M 28 Jan 2019 16:20:02.747 * Ready to accept connections
[    1.504370] clocksource: tsc: mask: 0xffffffffffffffff max_cycles: 0x2126dc50dfd, max_idle_ns: 440795251059 ns
INFO[0053] Caught signal terminated
WARN[0053] firecracker exited: signal: terminated
INFO[0053] Start machine was happy

```
There's no networking setup so its basically useless but you can run it. You'll also need to terminate the firecracker vm with `kill`.

Another caveat is scratch builds -- you'll need to set the init boot arg directly -- since the filesystem generated is basically empty. For example the hello-world container:

```sh
sudo docker-to-firecracker run --boot-init-arg=/hello --generate-boot-init=false docker.io/library/hello-world:latest
```

The flags passed to docker-to-firecracker tell it not to generate an init script and instead just call `/hello` directly.

