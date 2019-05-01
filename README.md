# Hostmgr - SSH host-based authentication in dynamic environments

Hostmgr manages SSH server-side configuration files to enable host-based authentication in dynamic environments (e.g., container orchestration).

## Running hostmgr

*Hostmgr* should normally be run by root as a service deamon. The following command-line arguments are available:

| argument       | description                                                                                   |
| -------------- | --------------------------------------------------------------------------------------------- |
| `--client`     | operate in client mode                                                                        |
| `--server`     | operate in server mode (default)                                                              |
| `--host xxx`   | the url of the [Redis](https://redis.io) server used to sync state between clients and server |
| `--filter xxx` | the redis key prefix for all entries on redis (defaults to `/hostmgr`)                        |
| `--user xxx`   | the user account running `ssh` on the client machine (defaults to `whoami`)                   |

Host-authentication is usually used in situations where a program authenticates on a server on behalf of a user. Hence, the client user that runs `ssh` is usually either root or a service account (running some user-facing service such as a webserver).

A typical client is started with

```bash
hostmgr --host redis://redis:6379 --filter /my-host-group --client --user apache
```

To start a associated server use

```bash
hostmgr --host redis://redis:6379 --filter /my-host-group
```

> Server and client modes can be mixed by specifying `--client` and `--server`.

### Dependencies

*Hostmgr* requires a [Redis server](https://redis.io) to store and communicate its state. A simple redis server can be started using Docker with 

```bash
docker run -d -p 6379:6379 redis
```

## Background

SSH has the ability to authenticate users based on the host they are connecting from. This is feature is usually disabled by default, and [a number of configuration steps](https://en.wikibooks.org/wiki/OpenSSH/Cookbook/Host-based_Authentication) are needed for users to be authenticated on the basis of the machine they are connecting from (rather than password or personal rsa keys).

### Client-side configuration

#### ssh_config

The client has to enable host-based authentication by adding the follwing to the `/etc/ssh/ssh_config` file:

```ssh
Host *
  HostbasedAuthentication yes
  StrictHostKeyChecking no
  EnableSSHKeysign yes
```

This can be restricted by host as well using a less permissive wildcard. It also disables strict host-key checking, because servers can also change at any time in a dynamic environment.

The client also needs to have access to [ssh-keysign](http://man.openbsd.org/ssh-keysign.8), which is a tool that is usually installed with the normal ssh client suite.

#### Host keys

You can usually find the private host key in `/etc/ssh/ssh_host_xxx_key` on machines that have the ssh server deamon (`sshd`) installed, whhere xxx is the key algorithm. The public key is stored in a file with the same name and the ending `.pub`. Machines without `sshd` often do not have host keys. There are two options:

1) Install (and deinstall) the ssh server package 
2) Generate the host keys manually

The best way to install the server package depends on your operating system. To generate host keys manually use as root

```bash
ssh-keygen -t rsa -C "comment" -N "" -f /etc/ssh/ssh_host_rsa_key
```

### Server-side configuration

The server-side configuration requires information in a number of places. This can include:

- sshd_config
- shosts.equiv, .shosts, .rhosts
- ssh_known_hosts, known_hosts

Multiple files per bullet point are largely equivalent and have similar syntax.

#### sshd_config

To enable host-based authentication, add the following to `/etc/ssh/sshd_config`:

```ssh
HostbasedAuthentication yes
HostbasedUsesNameFromPacketOnly yes
```

The second directive circumvents DNS reverse lookup of hostnames, which can be a problem in container environments. If your DNS setup works fine, this directive can be obmitted.

#### shosts.equiv

 First, the connecting host/user combinations need to be entered in `/etc/ssh/shosts.equiv`:

```shosts
client.example.com root
192.0.2.102 root
client8.example.com
```

Note that the second argument on each line in **not** the username of the account trying to connect (e.g., by issuing `ssh me@server.example.com`). It is the username of the account running the ssh command, which often is root or a service account in situations where host-based authentication is encountered.

> General SSH documentation suggests that a line only specifying the host should enable all users to connect irrespective of their username. The OpenSSH implementation that we have tested does not seem to allow for this feature, due to the implementation of the [auth_rhost2](https://github.com/openssh/openssh-portable/blob/master/auth-rhosts.c) routine.

#### ssh_known_hosts

This file stores the public keys of hosts mentioned in `shosts.equiv`. This is used to authenticate the hosts identity claim.

```known_hosts
desktop,192.0.2.102 ssh-rsa AAAAB3NzaC1yc2EAAAABIw ... qqU24CcgzmM=
```

Each line identifies the client machine by hostname, IP and its public key (usually found in `/usr/ssh/ssh_host_rsa_key.pub` or similar).

### Dynamic environments

In dynamic environments hostnames and IP addresses often change, which makes host-based ssh authentication difficult. *Hostmgr* solves that problem by updating the server-side configuration files (`shosts.equiv` and `ssh_known_hosts`) when the computing environment changes.

*Hostmgr* needs to run on each server and each host that tries to connect to the servers.

- In *server mode* (`--server` argument - the default), *hostmgr* listens for changes to the computing environment and updates the ssh configuration on the machine it is running.
- In *client mode* (`--client` argument), *hostmgr* announces the machine it is running on to listening servers and revokes the host when *hostmgr* stopped.


## Deployment

The recommended way to deploy *hostmgr* in a container is using the Docker build container feature:

```Dockerfile
FROM golang:1.10

RUN git clone https://github.com/kramergroup/hostmgr.git /go/src/github.com/kramergroup/hostmgr
WORKDIR /go/src/github.com/kramergroup/hostmgr
RUN CGO_ENABLED=1 go build -o hostmgr cmd/hostmgr/main.go

FROM ubuntu:disco
RUN apk add --no-cache ca-certificates
COPY --from=0 /go/src/github.com/kramergroup/hostmgr /bin/hostmgr
RUN mkdir /etc/ssh
ENTRYPOINT ["/bin/hostmgr"]
```

## Tutorial

The `docker-composite.yaml` file defines a test environment and sample deployment. It defines three container:

- server (defined by `Dockerfile.server` in `examples`) acts as a managed ssh server
- client (defined by `Dockerfile.client` in `examples`) provides a advertised client
- redis is a containerised redis server

1. The environment can be started with

```bash
docker-compose up
```

This will compile and start the server and client containers in addition to the redis server.

2. Access the server with

```bash
docker-compose exec server /bin/bash
```

and create a new user

```bash
useradd -m -p "test" test
```

> Note that users need a password even if it is not used for authentication via ssh. Otherwise, the user is marked as invalid

3. Access the client with

```bash
docker-compose exec server /bin/bash
```

and open a new ssh session to the server with

```bash
ssh test@server
```
