# This script automates and enhances part of the process described at
# https://goteleport.com/docs/get-started/deploy-community/. You should
# have certificates set up via mkcert as described at the beginning of
# this page.

let cluster_namespace = "local"
let proxy_hostname = $"($cluster_namespace)-teleport"
let proxy_port = "3080"
let teleport_bin_dir = "~/projects/teleport/adamkpickering/db-aup-revocation/build" | path expand

def "main up" [--type: string] {
  if $cluster_namespace not-in (in-use-docker-networks) {
    print $"creating docker network \"($cluster_namespace)\"..."
    docker network create --driver bridge $cluster_namespace | ignore
  }

  match $type {
    "teleport" =>      (ensure-teleport)
    "discovery" =>     (ensure-discovery-service)
    "postgres" =>      (ensure-postgres; ensure-db-resource --type $type)
    "mysql" =>         (ensure-mysql; ensure-db-resource --type $type)
    "mariadb" =>       (ensure-mariadb; ensure-db-resource --type $type)
    "mongodb" =>       (ensure-mongodb; ensure-db-resource --type $type)
    "cockroachdb" =>   (ensure-cockroachdb; ensure-db-resource --type $type)
    "redis" =>         (ensure-redis; ensure-db-resource --type $type)
    "redis-cluster" => (ensure-redis-cluster; ensure-db-resource --type $type)
    _ =>               (error make {msg: $"invalid --type ($type)"})
  }
}

def "main down" [--type: string] {
  match $type {
    "teleport" =>      (wipe-teleport)
    "discovery" =>     (wipe-discovery-service)
    "postgres" =>      (wipe-db-resource --type $type; wipe-postgres)
    "mysql" =>         (wipe-db-resource --type $type; wipe-mysql)
    "mariadb" =>       (wipe-db-resource --type $type; wipe-mariadb)
    "mongodb" =>       (wipe-db-resource --type $type; wipe-mongodb)
    "cockroachdb" =>   (wipe-db-resource --type $type; wipe-cockroachdb)
    "redis" =>         (wipe-db-resource --type $type; wipe-redis)
    "redis-cluster" => (wipe-db-resource --type $type; wipe-redis-cluster)
    _ =>               (error make {msg: $"invalid --type ($type)"})
  }

  if $cluster_namespace not-in (in-use-docker-networks) {
    print $"removing docker network \"($cluster_namespace)\"..."
    docker network rm --force $cluster_namespace | ignore
  }
}

def main [] {}

def in-use-docker-networks [] {
  docker container ls --all --format json |
    from json --objects |
    get Networks |
    uniq
}

def check-binaries [] {
  # check that we aren't going to try to build with macOS binaries
  if (file ([$teleport_bin_dir "teleport"] | path join)) =~ "Darwin" {
    error make {msg: "Are you sure you want to run Darwin build of teleport?"}
  }
}

def check-host-utilities [] {
  for utility in [oathtool zbarimg] {
    if (which $utility | is-empty) {
      error make {msg: $"required utility not found: ($utility) — install with: brew install oath-toolkit zbar"}
    }
  }
}

def ensure-teleport [] {
  let image_name = $proxy_hostname

  check-binaries
  check-host-utilities

  cd teleport

  # generate cert-related files; we assume that mkcert -install has already been run
  print "generating certs..."
  mkcert localhost e>| ignore
  mkcert $image_name e>| ignore
  
  # template out teleport.yaml
  print "templating out teleport.yaml..."
  let teleport_yaml = 'version: v3

teleport:
  nodename: 8d71dd3b74db
  data_dir: /var/lib/teleport
  join_params:
    token_name: ""
    method: token
  log:
    output: stderr
    severity: DEBUG
    format:
      output: text
  ca_pin: ""
  diag_addr: ""

auth_service:
  enabled: true
  listen_addr: 0.0.0.0:3025
  cluster_name: localhost
  proxy_listener_mode: multiplex
  authentication:
    type: local
    second_factors: ["webauthn", "otp"]
    connector_name: local
    webauthn:
      rp_id: localhost

ssh_service:
  enabled: true

db_service:
  enabled: true
  resources:
  - labels:
      "dev.local/managed": "true"

proxy_service:
  enabled: true
  web_listen_addr: 0.0.0.0:PORT_SUBSTITUTE
  public_addr:
  - localhost:PORT_SUBSTITUTE
  - IMAGE_NAME:PORT_SUBSTITUTE
  https_keypairs:
  - key_file: /etc/teleport-tls/localhost-key.pem
    cert_file: /etc/teleport-tls/localhost.pem
  - key_file: /etc/teleport-tls/IMAGE_NAME-key.pem
    cert_file: /etc/teleport-tls/IMAGE_NAME.pem
  https_keypairs_reload_interval: 0s
  acme: {}'
  $teleport_yaml | str replace --all IMAGE_NAME $image_name | str replace --all PORT_SUBSTITUTE $proxy_port | save --force teleport.yaml

  print "templating out Dockerfile..."
  let dockerfile = "FROM ubuntu:24.04

# Installing curl is critical because it sets up a directory we
# later use for certificates
RUN apt-get update && apt-get install -y curl

# This is used to make sudo simply execute the command
RUN printf '#!/bin/sh\\nexec \"$@\"\\n' > /usr/local/bin/sudo && chmod +x /usr/local/bin/sudo

COPY teleport.yaml /etc/teleport.yaml
COPY --from=mkcertca rootCA.pem /etc/ssl/certs/mkcertCA.pem
COPY localhost-key.pem /etc/teleport-tls/localhost-key.pem
COPY localhost.pem /etc/teleport-tls/localhost.pem
COPY IMAGE_NAME-key.pem /etc/teleport-tls/IMAGE_NAME-key.pem
COPY IMAGE_NAME.pem /etc/teleport-tls/IMAGE_NAME.pem
COPY --from=bins teleport /usr/local/bin/teleport
COPY --from=bins tsh /usr/local/bin/tsh
COPY --from=bins tctl /usr/local/bin/tctl

CMD [\"teleport\", \"start\"]"
  $dockerfile | str replace --all IMAGE_NAME $image_name | save --force Dockerfile

  # build main docker image
  print "building docker image..."
  docker build --quiet --build-context $"bins=($teleport_bin_dir)" --build-context $"mkcertca=(mkcert -CAROOT)" --tag $image_name . | ignore

  # ensure main container is running and set up
  print $"starting ($image_name) container..."
  docker run --quiet --detach --network $cluster_namespace --publish $"($proxy_port):($proxy_port)" --name $image_name $image_name | ignore

  cd ..

  sleep 3sec # give teleport some time to start up before creating users
  create-teleport-user teleport-admin
}

def wipe-teleport [] {
  let image_name = $proxy_hostname

  print "wiping teleport containers..."
  wipe-containers $image_name
  print "wiping teleport images..."
  wipe-images $image_name

  cd teleport
  print "wiping teleport directory..."
  rm --force *.pem Dockerfile teleport.yaml teleport-admin.identity
  cd ..
}

def create-teleport-user [username: string, password: string = "asdfasdfasdf"] {
  let image_name = $proxy_hostname
  let proxy = $"localhost:($proxy_port)"

  print $"adding ($username) user..."
  let out = (docker exec $image_name sudo tctl users add $username
               --roles=editor,access,auditor
               --logins=root,ubuntu
               --db-users='*' --db-names='*' | complete)
  let token = ($out.stdout | parse --regex 'invite/(?<t>\w+)' | get t.0)

  print $"fetching TOTP registration challenge for ($username)..."
  let challenge = (http post $"https://($proxy)/v1/webapi/mfa/token/($token)/registerchallenge"
    --content-type application/json
    {deviceType: "totp", deviceUsage: "mfa"})

  $challenge.totp.qrCode | decode base64 | save --force /tmp/tp-qr.png
  let otpauth = (zbarimg --quiet --raw /tmp/tp-qr.png | str trim)
  let secret = ($otpauth | parse --regex 'secret=(?<s>[A-Z2-7]+)' | get s.0)

  let code = (oathtool --totp -b $secret | str trim)

  print $"completing registration for ($username)..."
  (http put $"https://($proxy)/v1/webapi/users/password/token"
    --content-type application/json
    {
      token: $token
      password: ($password | encode base64)
      second_factor_token: $code
      device_name: "scripted-totp"
    })

  print $"creating identity file for ($username)..."
  let container_identity_path = $"/tmp/($username).identity"
  let local_identity_path = $"teleport/($username).identity"
  docker exec $image_name sudo tctl auth sign --user $username --out $container_identity_path --format=file --ttl=2160h o+e>| ignore
  docker cp $"($image_name):($container_identity_path)" $local_identity_path | ignore

  print $"created user ($username); use identity file ($local_identity_path) to login"
}

def ensure-redis-cluster [] {
  let image_name = $"($cluster_namespace)-redis-cluster"
  let node_names = [
    $"($image_name)-node-1"
    $"($image_name)-node-2"
    $"($image_name)-node-3"
    $"($image_name)-node-4"
    $"($image_name)-node-5"
    $"($image_name)-node-6"
  ]

  cd redis-cluster

  print "generating root CA cert for redis-cluster..."
  open ssl.conf.tpl |
    str replace --all "SANS_SUBSTITUTE" "DNS:localhost,IP:127.0.0.1" |
    save --force ssl-ca.conf
	openssl genrsa -out ca.key 2048 | complete | ignore
	openssl req -config ssl-ca.conf -key ca.key -new -x509 -days 365 -sha256 -extensions v3_ca -subj "/CN=ca" -out ca.crt | complete | ignore

  print "exporting teleport db_client CA cert..."
  tctl auth export --type=db-client | save --force db-client-ca.crt
  cat ca.crt db-client-ca.crt | save --force combined.crt

  for node_name in $node_names {
    print $"copying common files for ($node_name)..."
    mkdir $node_name
    cp combined.crt $"($node_name)/combined.crt"
    cp redis.conf $"($node_name)/redis.conf"
    cp users.acl $"($node_name)/users.acl"
    cp Dockerfile $"($node_name)/Dockerfile"
    open ssl.conf.tpl |
      str replace --all "SANS_SUBSTITUTE" $"DNS:($node_name),DNS:localhost,IP:127.0.0.1" |
      save --force $"($node_name)/ssl.conf"

    cd $node_name

    print $"setting up certs for ($node_name)..."
    openssl genrsa -out node.key 2048 | complete | ignore
    openssl req -config ssl.conf -subj $"/CN=($node_name)" -key node.key -new -out node.csr | complete | ignore
    openssl x509 -req -in node.csr -CA ../ca.crt -CAkey ../ca.key -CAcreateserial -days 365 -out node.crt -extfile ssl.conf -extensions server_and_client_cert | complete | ignore

    print $"building image for ($node_name)..."
    docker build --quiet --tag $node_name . | ignore

    print $"starting container for ($node_name)..."
    docker run --quiet --detach --network $cluster_namespace --name $node_name $node_name | ignore

    cd ..
  }

  # Give redis nodes some time to start up
  sleep 5sec

  print "generating client certificate for redis-cli..."
  let bootstrap_container_name = $"($image_name)-bootstrap"
  open ssl.conf.tpl |
    str replace --all "SANS_SUBSTITUTE" $"DNS:($bootstrap_container_name),DNS:localhost,IP:127.0.0.1" |
    save --force ssl-client.conf
  openssl genrsa -out client.key 2048 | complete | ignore
  openssl req -config ssl-client.conf -subj "/CN=redis-cli-client" -key client.key -new -out client.csr | complete | ignore
  openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 365 -out client.crt -extfile ssl-client.conf -extensions client_cert | complete | ignore

  print "running cluster create command..."
  let node_addrs = $node_names | each { |n| $"($n):6379" }
  (docker run --rm -it --network $cluster_namespace --name $bootstrap_container_name -v $"(PWD):/tls:ro" redis:7.2.3
    redis-cli --tls --cacert /tls/ca.crt --cert /tls/client.crt --key /tls/client.key
    --user alice -a test --cluster create
    ...$node_addrs
    --cluster-replicas 1 --cluster-yes | ignore)

  cd ..
}

def wipe-redis-cluster [] {
  let image_name = $"($cluster_namespace)-redis-cluster"

  print "wiping redis-cluster containers..."
  wipe-containers $image_name
  print "wiping redis-cluster images..."
  wipe-images $image_name

  cd redis-cluster
  print "wiping redis-cluster directory..."
  glob $"($image_name)-node-*" | each {|it| rm --force --recursive $it} | ignore
  rm --force ca.crt ca.key ca.srl combined.crt db-client-ca.crt
  rm --force client.crt client.csr client.key
  rm --force ssl-ca.conf
  rm --force ssl-client.conf
  cd ..
}

def ensure-redis [] {
  let image_name = $"($cluster_namespace)-redis"

  print "generating redis certificates..."
  cd redis
  tctl auth sign --format=redis --host=($image_name) --out=redis --ttl=2160h | ignore

  print "building redis image..."
  docker build --quiet --tag $image_name . | ignore

  print "starting redis container..."
  docker run --quiet --detach --network $cluster_namespace --name $image_name $image_name | ignore
  cd ..
}

def wipe-redis [] {
  let image_name = $"($cluster_namespace)-redis"

  print "wiping redis containers..."
  wipe-containers $image_name
  print "wiping redis images..."
  wipe-images $image_name

  cd redis
  print "wiping redis certs..."
  rm --force redis.cas redis.crt redis.key
  cd ..
}

def ensure-cockroachdb [] {
  let image_name = $"($cluster_namespace)-cockroachdb"

  print "generating cockroachdb certificates..."
  cd cockroachdb
  mkdir certs
  tctl auth sign --format=cockroachdb --host=($image_name),127.0.0.1 --out=certs --ttl=2160h | ignore

  print "building cockroachdb image..."
  docker build --quiet --tag $image_name . | ignore

  print "starting cockroachdb container..."
  docker run --quiet --detach --network $cluster_namespace --name $image_name $image_name | ignore
  cd ..
}

def wipe-cockroachdb [] {
  let image_name = $"($cluster_namespace)-cockroachdb"

  print "wiping cockroachdb containers..."
  wipe-containers $image_name
  print "wiping cockroachdb images..."
  wipe-images $image_name

  cd cockroachdb
  print "wiping cockroachdb certs..."
  rm --force --recursive certs
  cd ..
}

def ensure-mongodb [] {
  let image_name = $"($cluster_namespace)-mongodb"

  print "generating mongodb certificates..."
  cd mongodb
  tctl auth sign --format=mongodb --host=($image_name) --out=mongodb --ttl=2160h | ignore

  print "building mongodb image..."
  docker build --quiet --tag $image_name . | ignore

  print "starting mongodb container..."
  docker run --quiet --detach --network $cluster_namespace --name $image_name --user mongodb $image_name | ignore
  cd ..
}

def wipe-mongodb [] {
  let image_name = $"($cluster_namespace)-mongodb"

  print "wiping mongodb containers..."
  wipe-containers $image_name
  print "wiping mongodb images..."
  wipe-images $image_name

  cd mongodb
  print "wiping mongodb certs..."
  rm --force mongodb.cas mongodb.crt mongodb.key
  cd ..
}

def ensure-mariadb [] {
  let image_name = $"($cluster_namespace)-mariadb"

  print "generating mariadb certificates..."
  cd mariadb
  tctl auth sign --format=db --host=($image_name) --out=mariadb --ttl=2160h | ignore

  print "building mariadb image..."
  docker build --quiet --tag $image_name . | ignore

  print "starting mariadb container..."
  docker run --quiet --detach --network $cluster_namespace --name $image_name --user mysql $image_name | ignore
  cd ..
}

def wipe-mariadb [] {
  let image_name = $"($cluster_namespace)-mariadb"

  print "wiping mariadb containers..."
  wipe-containers $image_name
  print "wiping mariadb images..."
  wipe-images $image_name

  cd mariadb
  print "wiping mariadb certs..."
  rm --force mariadb.cas mariadb.crt mariadb.key
  cd ..
}

def ensure-mysql [] {
  let image_name = $"($cluster_namespace)-mysql"

  print "generating mysql certificates..."
  cd mysql
  tctl auth sign --format=db --host=($image_name) --out=mysql --ttl=2160h | ignore

  print "building mysql image..."
  docker build --quiet --tag $image_name . | ignore

  print "starting mysql container..."
  docker run --quiet --detach --network $cluster_namespace --name $image_name --user mysql $image_name | ignore
  cd ..
}

def wipe-mysql [] {
  let image_name = $"($cluster_namespace)-mysql"

  print "wiping mysql containers..."
  wipe-containers $image_name
  print "wiping mysql images..."
  wipe-images $image_name

  cd mysql
  print "wiping mysql certs..."
  rm --force mysql.cas mysql.crt mysql.key
  cd ..
}

def ensure-postgres [] {
  let image_name = $"($cluster_namespace)-postgres"

  cd postgres

  print "templating out pg_hba.conf..."
  let pg_hba_conf = "# TYPE  DATABASE  USER  ADDRESS    METHOD
local   all       all              trust
hostssl all       all   ::/0       cert
hostssl all       all   0.0.0.0/0  cert
"
  $pg_hba_conf | save --force pg_hba.conf

  print "generating postgres certificates..."
  tctl auth sign --format=db --host=($image_name) --out=postgres --ttl=2160h | ignore

  print "building postgres image..."
  docker build --quiet --tag $image_name . | ignore

  print $"starting postgres container..."
  (docker run --quiet --detach --network $cluster_namespace --name $image_name $image_name
    postgres
    -c ssl=on
    -c ssl_cert_file=/pg-certs/postgres.crt
    -c ssl_key_file=/pg-certs/postgres.key
    -c ssl_ca_file=/pg-certs/postgres.cas
    -c hba_file=/etc/postgresql/pg_hba.conf
    -c listen_addresses=* | ignore)

  cd ..
}

def wipe-postgres [] {
  let image_name = $"($cluster_namespace)-postgres"

  print "wiping postgres containers..."
  wipe-containers $image_name
  print "wiping postgres images..."
  wipe-images $image_name

  cd postgres
  print "wiping postgres certs and config..."
  rm --force postgres.cas postgres.crt postgres.key pg_hba.conf
  cd ..
}

def ensure-db-resource [--type: string] {
  let db_protocol = match $type {
    "mariadb" => "mysql"
    "redis-cluster" => "redis"
    _ => $type
  }
  let db_uri = match $type {
    "postgres" => $"($cluster_namespace)-postgres:5432"
    "mysql" => $"($cluster_namespace)-mysql:3306"
    "mariadb" => $"($cluster_namespace)-mariadb:3306"
    "mongodb" => $"($cluster_namespace)-mongodb:27017"
    "cockroachdb" => $"($cluster_namespace)-cockroachdb:26257"
    "redis" => $"($cluster_namespace)-redis:6379"
    "redis-cluster" => $"($cluster_namespace)-redis-cluster-node-1:6379"
    _ => (error make {msg: $"invalid type ($type)"})
  }
  mut db_resource = {
    kind: db
    version: v3
    metadata: {
      name: $type
      labels: {
        "dev.local/managed": "true"
        database_type: $type
      }
    }
    spec: {
      protocol: $db_protocol
      uri: $db_uri
    }
  }
  if $type in [postgres mysql mariadb mongodb] {
    $db_resource.spec.admin_user = {name: "teleport-admin"}
  }
  if $type == "redis-cluster" {
    $db_resource.spec.tls = {ca_cert: (open $"($type)/ca.crt")}
  }

  print $"creating db resource for ($type)..."
  $db_resource | to yaml | tctl create -f /dev/stdin | ignore
}

def wipe-db-resource [--type: string] {
  print $"removing db resource for ($type)..."
  tctl rm $"db/($type)" | ignore
}

def ensure-discovery-service [] {
  let image_name = $"($cluster_namespace)-discovery-service"

  check-binaries

  cd discovery-service

  print $"templating out teleport.yaml for discovery service..."
  let join_token = (tctl tokens add --type=db,node,discovery --ttl=100h --format=text)
  mut teleport_yaml = {
    version: v3
    teleport: {
      nodename: "discovery-service"
      join_params: {
        token_name: $join_token
        method: token
      },
      proxy_server: $"($proxy_hostname):($proxy_port)"
      log: {
        output: stderr
        severity: DEBUG
        format: {
          output: text
        }
      }
    },
    auth_service: {
      enabled: false
    }
    proxy_service: {
      enabled: false
    }
    ssh_service: {
      enabled: true
    }
    db_service: {
      enabled: true
      resources: [
        {
          labels: {
            "teleport.dev/creator": "adam.pickering@goteleport.com"
          }
          aws: {
            assume_role_arn: "arn:aws:iam::278576220453:role/adamkpickering-db-access-20260423182903605300000003"
          }
        }
      ]
    }
    discovery_service: {
      enabled: true
      aws: [
        {
          types: ["rds"]
          regions: ["us-west-2"]
          tags: {
            "teleport.dev/creator": "adam.pickering@goteleport.com"
          }
          assume_role_arn: "arn:aws:iam::278576220453:role/adamkpickering-db-discovery-20260423182903605300000002"
        }
      ]
    }
  }
  $teleport_yaml | save --force teleport.yaml

  print "building discovery service image..."
  docker build --quiet --build-context $"mkcertca=(mkcert -CAROOT)" --build-context $"bins=($teleport_bin_dir)" --tag $image_name . | ignore

  print $"starting discovery service container..."
  # Note: TELEPORT_TUNNEL_PUBLIC_ADDR is needed for the case where the proxy/auth services
  # are deployed in a local docker container, and are accessed via both localhost (via forwarded
  # ports) and the container name (for database service containers).
  docker run --quiet --detach --env $"TELEPORT_TUNNEL_PUBLIC_ADDR=($proxy_hostname):3080" --env-file teleport.env --network $cluster_namespace --name $image_name $image_name | ignore

  cd ..
}

def wipe-discovery-service [] {
  let image_name = $"($cluster_namespace)-discovery-service"

  print "wiping discovery service containers..."
  wipe-containers $image_name
  print "wiping discovery service images..."
  wipe-images $image_name

  cd discovery-service
  rm --force teleport.yaml
  cd ..
}

def wipe-containers [image_name: string] {
  docker container ls --all --format json | from json --objects | where Image =~ $image_name | each {|it|
    docker container stop $it.ID | ignore;
    docker container rm $it.ID | ignore
  } | ignore
}

def wipe-images [image_name: string] {
  docker image ls --format json | from json --objects | where Repository =~ $image_name | each {|it|
    docker image rm $it.ID
  } | ignore
}
