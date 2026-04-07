# dbtest

A CLI tool for spinning up test databases alongside Teleport database services,
for use in manual testing of Teleport's database access integrations.

Each database runs in a Docker container. Each database gets its own Teleport
database service container, which is configured to join an existing Teleport
cluster and register the database with it. Once both containers are running,
you can connect to the database through Teleport using `tsh db connect` as you
normally would.

## Concepts

### Cluster

A **cluster** is the top-level unit of organization. It maps to:

- A named Docker bridge network that all containers in the cluster share.
- A directory under `~/.teleport-dbtest/<name>/` that stores all state for
  the cluster, including certificates, config files, and Teleport binaries
  used to build images.
- A reference to an existing Teleport cluster (via its proxy server address).
  This tool does not create or manage the Teleport cluster itself — it only
  joins things to one that already exists.

### Database

A **database** is a specific database engine instance (e.g. PostgreSQL, MySQL)
running within a cluster. Bringing a database up does the following:

1. Generates TLS certificates for the database via `tctl auth sign`.
2. Builds a Docker image that embeds those certificates and any required config.
3. Starts the database container on the cluster's Docker network.
4. Generates a join token for a database service via `tctl tokens add`.
5. Builds and starts a Teleport database service container configured to join
   the Teleport cluster and proxy the database, using the Teleport binaries
   stored in the cluster's state directory.

## Prerequisites

- Docker
- `tctl` and `tsh` in `$PATH`, authenticated to the Teleport cluster you want
  to use
- `openssl` in `$PATH` (required for redis-cluster only)
- A local directory containing Linux Teleport enterprise binaries (`teleport`,
  `tsh`, `tctl`) — used when creating a cluster

## Building

```
go build -o dbtest .
```

## Usage

### Clusters

**Create a cluster:**
```
dbtest cluster create mydev \
    --proxy-server my-cluster.teleport.example.com:443 \
    --binaries-dir /path/to/linux/teleport-binaries
```

The `--binaries-dir` directory must contain Linux `teleport`, `tsh`, and `tctl`
binaries. They are copied into the cluster's state directory at creation time
and reused for every database service built in that cluster.

The cluster name becomes the Docker network name and the prefix for all
container and image names belonging to the cluster.

**List all clusters:**
```
dbtest cluster list
```

```
mydev   my-cluster.teleport.example.com:443
```

**Get details for a cluster:**
```
dbtest cluster get mydev
```

```
Name:         mydev
Proxy Server: my-cluster.teleport.example.com:443
Binaries:     /Users/you/.teleport-dbtest/mydev/teleport-binaries
Directory:    /Users/you/.teleport-dbtest/mydev
```

**Remove a cluster** (wipes all associated Docker containers, images, and
network, and removes all state):
```
dbtest cluster remove mydev
```

### Databases

**Start a database:**
```
dbtest db up --cluster mydev --name postgres --type postgres
dbtest db up --cluster mydev --name mysql --type mysql
dbtest db up --cluster mydev --name redis --type redis
```

The `--name` flag sets the instance name, which becomes part of the container
name (`<cluster>-<name>`). Multiple instances of the same type can coexist in
a cluster by giving them different names.

Supported types: `postgres`, `mysql`, `mariadb`, `mongodb`, `cockroachdb`,
`redis`, `redis-cluster`.

**Stop and remove a database:**
```
dbtest db down --cluster mydev --name postgres --type postgres
```

This stops and removes the database and database service containers and images,
and deletes the associated files from the cluster state directory.

Pass `--debug` to either command to print the output of underlying docker and
tctl commands.

### Full workflow example

```sh
# 1. Create a cluster backed by your Teleport cluster.
dbtest cluster create mydev \
    --proxy-server my-cluster.teleport.example.com:443 \
    --binaries-dir /path/to/linux/teleport-ent

# 2. Start a PostgreSQL database and its database service.
dbtest db up --cluster mydev --name postgres --type postgres

# 3. Connect via Teleport (assuming a suitable role is configured).
tsh db connect --db-user=alice --db-name=postgres mydev-postgres

# 4. Add another database to the same cluster.
dbtest db up --cluster mydev --name mongo --type mongodb

# 5. Tear down just MongoDB when you're done with it.
dbtest db down --cluster mydev --name mongo --type mongodb

# 6. Tear down everything when finished.
dbtest cluster remove mydev
```

## State directory layout

All cluster state lives under `~/.teleport-dbtest/`:

```
~/.teleport-dbtest/
└── mydev/
    ├── cluster.json                  # cluster config
    ├── teleport-binaries/            # Linux Teleport binaries (copied at cluster create)
    │   ├── teleport
    │   ├── tsh
    │   └── tctl
    ├── postgres/                     # files used to build the postgres image
    │   ├── Dockerfile
    │   ├── postgres.crt
    │   ├── postgres.key
    │   └── postgres.cas
    ├── postgres-database-service/    # config for the postgres database service
    │   ├── Dockerfile
    │   ├── teleport.yaml
    │   └── ca.crt
    ├── mysql/
    │   └── ...
    └── ...
```

## Options

**`cluster create`**

| Flag | Default | Description |
|------|---------|-------------|
| `--proxy-server` | *(required)* | Teleport proxy address (`host:port`). Also read from `$TELEPORT_PROXY_SERVER`. |
| `--binaries-dir` | *(required)* | Path to a local directory containing Linux Teleport binaries (`teleport`, `tsh`, `tctl`). |

**`db up` / `db down`**

| Flag | Default | Description |
|------|---------|-------------|
| `--cluster` | *(required)* | Cluster name. |
| `--name` | *(required)* | Database instance name (forms the container name `<cluster>-<name>`). |
| `--type` | *(required)* | Database type (`postgres`, `mysql`, `mariadb`, `mongodb`, `cockroachdb`, `redis`, `redis-cluster`). |
| `--debug` | `false` | Print output of docker/tsh/tctl subcommands. |
