---
name: teleport-dbtest
description: Spin up a local Teleport cluster with one or more databases for testing database access features. Use when asked to test Teleport database access, auto user provisioning, database permissions, or any scenario involving tsh/tctl and a real database. Requires a directory of Teleport binaries as input.
tools: Bash, Read, Edit, Write
---

# teleport-dbtest — Teleport Database Testing

This skill provisions a short-lived local Teleport cluster and one or more databases inside Docker, lets you run whatever tests are needed, then tears everything down cleanly.

## Prerequisites

The host machine must have:
- Docker
- nushell (`nu`)
- `mkcert` (with `mkcert -install` already run once)
- `openssl`
- `tsh` / `tctl` on PATH **or** the agent uses the ones from the bin dir after login

## Inputs required before starting

You must have two pieces of information before doing anything else:

1. **`teleport_bin_dir`** — absolute path to a directory containing Linux `teleport`, `tsh`, and `tctl` binaries. Confirm the path exists and contains those three files before proceeding.
2. **Purpose of the run** — a short description of what you are testing (e.g. "auto user provisioning best-effort-drop", "redis cluster TLS"). You will use this to derive `cluster_namespace` and choose a port.

If either is missing, ask the user before continuing.

## Step 0: Locate the repo and create a worktree

This skill is installed as a symlink: `~/.claude/skills/teleport-dbtest` → `<repo>/default/skill`. Use that symlink to find the repo:

```bash
SKILL_DIR="$(readlink -f ~/.claude/skills/teleport-dbtest)"
DEFAULT_WORKTREE="$(dirname "$SKILL_DIR")"       # <repo>/default
REPO_PARENT="$(dirname "$DEFAULT_WORKTREE")"     # <repo> (sibling dir for new worktrees)
```

**Never work directly in `$DEFAULT_WORKTREE`.** Instead, create a sibling worktree named after `cluster_namespace` (chosen in Step 1):

```bash
WORKTREE_PATH="$REPO_PARENT/$CLUSTER_NAMESPACE"
git -C "$DEFAULT_WORKTREE" worktree add -b "$CLUSTER_NAMESPACE" "$WORKTREE_PATH"
```

All subsequent file edits and `nu dev.nu` invocations must happen inside `$WORKTREE_PATH`. Never touch files outside this worktree.

## Step 1: Choose cluster_namespace and proxy_port

**`cluster_namespace` is your primary unique identifier for this run.** It must be used consistently everywhere a unique name is needed, including the worktree directory name, git branch name, Docker network name, and (implicitly) all container and image names, since `dev.nu` prefixes everything with it.

`cluster_namespace` must be:
- Unique across currently running clusters on this machine (check `docker network ls`)
- Short and descriptive (3–15 chars, lowercase, hyphens ok): e.g. `aup-drop`, `redis-tls`, `pg-perms`
- Derived from the purpose of the run, not random

**`proxy_port`** must be:
- A free TCP port on the host (check with `lsof -iTCP:<port> -sTCP:LISTEN` — no output means free)
- Distinct from any other running cluster's port
- Pick something in the 3081–3199 range to avoid conflicts with the `default` cluster (which uses 3080)

## Step 2: Edit dev.nu in the worktree — once, before anything starts

Open `$WORKTREE_PATH/dev.nu` and update the three variables at the very top:

```nushell
let cluster_namespace = "<chosen namespace>"
let proxy_port        = "<chosen port>"
let teleport_bin_dir  = "<absolute path to bin dir>" | path expand
```

**These three variables must not be changed again for the lifetime of this cluster.** Changing them after containers are running will break everything.

Also update `ensure-discovery-service` if you will use the discovery service: the hardcoded AWS ARNs and email addresses in that function belong to the project author and may need adjustment.

## Step 3: Bring up Teleport

```bash
cd "$WORKTREE_PATH"
nu dev.nu up --type teleport
```

This:
1. Creates the `<cluster_namespace>` Docker network
2. Generates mkcert TLS certs for `localhost` and `<cluster_namespace>-teleport`
3. Builds and starts the Teleport container (proxy + auth + db_service + ssh_service)
4. Prints a one-time user-invite URL — **capture it and complete setup via browser**

After the invite link is printed, the agent cannot complete the browser-based registration. Pause and ask the user to:
1. Open the invite URL in a browser
2. Register (set password + MFA)
3. Run `tsh login --proxy=localhost:<proxy_port> --user=teleport-admin` in their terminal

Once the user confirms they are logged in, continue.

### Important: tsh/tctl after login

After `tsh login`, the `tsh` and `tctl` on the host PATH will work for managing the cluster. The agent can run them directly. The in-cluster binaries (inside `teleport_bin_dir`) are Linux ELF binaries used inside Docker only — do not try to execute them directly on macOS.

## Step 4: Apply roles (optional but common)

The `roles/` directory contains pre-built role YAMLs. Apply whichever are relevant:

```bash
# Example: apply all roles
for f in "$WORKTREE_PATH/roles/"*.yaml; do
  tctl create -f "$f"
done

# Or one at a time:
tctl create -f "$WORKTREE_PATH/roles/aup-best-effort-drop.yaml"
```

Available roles:

| File | Mode | Grant mechanism |
|------|------|----------------|
| `aup-best-effort-drop.yaml` | `best_effort_drop` | db_roles: [creator] |
| `aup-keep.yaml` | `keep` | db_roles: [creator] |
| `dbpm-best-effort-drop.yaml` | `best_effort_drop` | db_permissions (table-level SELECT/CREATE) |
| `dbpm-keep.yaml` | `keep` | db_permissions (table-level SELECT/CREATE) |
| `specific-db-label.yaml` | static access | restricts to `database_type: postgres` label |

Assign roles to the `alice` Teleport user (the standard test user) with:

```bash
tctl users update alice --set-roles=access,<role-name>
# or create alice if needed:
tctl users add alice --roles=access,<role-name> --db-users='*' --db-names='*'
```

## Step 5: Bring up a database

```bash
cd "$WORKTREE_PATH"
nu dev.nu up --type <db>
```

Supported `--type` values: `postgres`, `mysql`, `mariadb`, `mongodb`, `cockroachdb`, `redis`, `redis-cluster`

This signs database certs via `tctl auth sign`, builds the DB Docker image, starts the container, and registers the `db` resource in Teleport.

### Database-specific notes

**postgres** — uses cert-based auth (`hostssl … cert`). The `teleport-admin` DB user is created as SUPERUSER by the init script (`scripts/postgres.sql`). The `creator` role is also created; grant it to test DB-level role assignment.

**mysql / mariadb** — use `tctl auth sign --format=db`. Init SQL in `mysql/init.sql` / `mariadb/init.sql` creates the `teleport-admin` user.

**mongodb** — uses `--format=mongodb`. The `teleport-admin` user must be created inside mongo after startup if not done by init.

**cockroachdb** — uses `--format=cockroachdb`. Needs `certs/` subdirectory (created by the script).

**redis** — single-node, uses `tctl auth sign --format=redis`.

**redis-cluster** — six-node cluster with its own CA. Requires an interactive `redis-cli --cluster create` step; the script runs it automatically but needs the nodes to be healthy first.

## Step 6: Connect and test

```bash
# List available databases
tsh db ls

# Log in to a specific database
tsh db login <db-name>

# Get connection info
tsh db config <db-name>

# Connect via tsh proxy (for postgres example):
tsh db connect <db-name>

# Or get the connection string and use a native client:
tsh db config --format=cmd <db-name>
```

### Testing auto user provisioning (AUP)

AUP creates and drops the connecting Teleport user inside the database automatically. To test:

1. Apply one of the `aup-*` roles and assign it to `alice`
2. Connect as alice: `tsh db connect <db-name> --db-user=alice`
3. Verify the user was created: inside the DB, check `\du` (postgres) or `SHOW GRANTS FOR 'alice'@'%'` (mysql)
4. Disconnect and verify the user was dropped (for `best_effort_drop` mode)

For postgres AUP the `creator` role must exist and `teleport-admin` must have ADMIN OPTION on it — `scripts/aup.sql` sets this up. Run it if needed:

```bash
tsh db connect postgres --db-user=teleport-admin --db-name=postgres < "$WORKTREE_PATH/scripts/aup.sql"
```

### Testing DB permissions management (DBPM)

DBPM grants/revokes fine-grained table permissions on connect/disconnect. To test:

1. Apply one of the `dbpm-*` roles and assign it to `alice`
2. Run the setup SQL to create test tables:
   ```bash
   tsh db connect postgres --db-user=teleport-admin --db-name=postgres < "$WORKTREE_PATH/scripts/db_permissions_testing.sql"
   ```
3. Connect as alice and verify SELECT works on `allowed_test_table` but not `disallowed_test_table`

## Step 7: Tear down

Always tear down in reverse order: databases first, then Teleport.

```bash
cd "$WORKTREE_PATH"
nu dev.nu down --type postgres   # repeat for each DB you started
nu dev.nu down --type teleport
```

Then remove the worktree:

```bash
git -C "$DEFAULT_WORKTREE" worktree remove "$WORKTREE_PATH"
git -C "$DEFAULT_WORKTREE" branch -d "$CLUSTER_NAMESPACE"
```

## Guardrails

- **Never edit files outside your worktree.** The `default/` directory and anything else outside `$WORKTREE_PATH` is off-limits.
- **Never change `cluster_namespace`, `proxy_port`, or `teleport_bin_dir` after the cluster is started.** If you realise you need different values, tear down and start fresh.
- **Never modify `teleport_bin_dir` contents.** That directory is read-only for the lifetime of the cluster.
- **Do not add new `ensure-*` or `wipe-*` functions to `dev.nu`.** If the existing functions don't cover what you need, stop and ask the user for guidance — you have probably misunderstood the task.
- **Clusters are short-lived.** Do not leave containers running after testing is complete.
- **Only one cluster per worktree.** Each worktree has its own `cluster_namespace` and port; do not try to run two logical clusters from the same worktree.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `Are you sure you want to run Darwin build` | `teleport_bin_dir` points to macOS binaries | Use a Linux build directory |
| `docker: Error response from daemon: Conflict` | Container name already in use | Run `nu dev.nu down --type <x>` first |
| `tctl: command not found` | Not logged in / tsh not on PATH | Run `tsh login` first |
| `tctl auth sign` fails | Teleport not fully started yet | Wait 5–10 s, retry |
| redis-cluster nodes unhealthy | Cluster create ran too early | Increase the `sleep 5sec` in `ensure-redis-cluster` or rerun cluster create manually |
| Port 3080 in use | Another cluster or leftover container | Pick a different `proxy_port` or stop the conflicting process |
