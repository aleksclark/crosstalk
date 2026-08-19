#!/usr/bin/env bash
# Fail-closed validation for CrossTalk docker-compose.dev.yml (Stacklane lifecycle).
# Uses `docker compose config --format json` into a mode-0700 temp file; never dumps full env/secrets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_SLUG="crosstalk"
COMPOSE_FILE_NAME="docker-compose.dev.yml"

die() { echo "compose-dev-check: FAIL: $*" >&2; exit 1; }
ok() { echo "compose-dev-check: ok: $*" >&2; }
info() { echo "compose-dev-check: $*" >&2; }

# Do not inherit an operator's compose file/profile/project accidentally.
unset COMPOSE_FILE COMPOSE_PROFILES COMPOSE_PROJECT_NAME || true
COMPOSE_FILE="${ROOT}/${COMPOSE_FILE_NAME}"

sanitize_instance() {
  local s="${1:-}"
  s="$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')"
  s="$(printf '%s' "$s" | sed -E 's/[^a-z0-9-]+/-/g; s/-+/-/g; s/^-+//; s/-+$//')"
  if [[ ${#s} -gt 48 ]]; then
    s="${s:0:48}"
    s="$(printf '%s' "$s" | sed -E 's/-+$//')"
  fi
  [[ -z "$s" ]] && s="dev"
  printf '%s' "$s"
}

derive_instance() {
  if [[ -n "${STACKLANE_INSTANCE:-}" ]]; then
    sanitize_instance "$STACKLANE_INSTANCE"
    return
  fi
  sanitize_instance "$(basename "$ROOT")"
}

require_tools() {
  command -v docker >/dev/null 2>&1 || die "docker not found"
  docker compose version >/dev/null 2>&1 || die "docker compose not available"
  command -v python3 >/dev/null 2>&1 || die "python3 required for JSON parse"
  [[ -f "$COMPOSE_FILE" ]] || die "missing $COMPOSE_FILE"
}

render_config() {
  local project="$1"
  local instance="$2"
  local out="$3"
  local base_dom="${STACKLANE_BASE_DOMAIN:-test}"
  umask 077
  : >"$out"
  chmod 600 "$out"
  if ! STACKLANE_INSTANCE="$instance" \
    STACKLANE_BASE_DOMAIN="$base_dom" \
    COMPOSE_PROJECT_NAME="$project" \
    timeout 60 docker compose -p "$project" --project-directory "$ROOT" -f "$COMPOSE_FILE" config --format json >"$out"; then
    die "compose config render failed (rule: render)"
  fi
  chmod 600 "$out"
}

run_mutation_probes() {
  local tmpdir="$1"
  local base_yml="$COMPOSE_FILE"
  python3 - "$base_yml" "$tmpdir" "$ROOT" <<'PY'
import json, os, pathlib, subprocess, sys

base_path = pathlib.Path(sys.argv[1])
tmpdir = pathlib.Path(sys.argv[2])
root = pathlib.Path(sys.argv[3])
src = base_path.read_text()

def write_mut(name, text):
    p = tmpdir / f"mut-{name}.yml"
    p.write_text(text)
    return p

mutations = [
    ("wildcard-host", src.replace('"127.0.0.1::8080"', '"0.0.0.0::8080"'), "host_ip must be 127.0.0.1"),
    ("fixed-host-port", src.replace('"127.0.0.1::8080"', '"127.0.0.1:18080:8080"'), "published port must be ephemeral"),
    ("missing-enable-label", src.replace('\n      stacklane.enable: "true"\n', '\n', 1), "stacklane.enable"),
]

failures = 0
for name, text, expect_hint in mutations:
    mut_path = write_mut(name, text)
    out = tmpdir / f"mut-{name}.json"
    env = os.environ.copy()
    env.pop("COMPOSE_FILE", None)
    env.pop("COMPOSE_PROFILES", None)
    env["STACKLANE_INSTANCE"] = "mutprobe"
    env["STACKLANE_BASE_DOMAIN"] = "test"
    env["COMPOSE_PROJECT_NAME"] = "crosstalk-mutprobe"
    try:
        with out.open("w") as fh:
            subprocess.run(
                ["docker", "compose", "-p", "crosstalk-mutprobe",
                 "--project-directory", str(root), "-f", str(mut_path),
                 "config", "--format", "json"],
                check=True, env=env, stdout=fh, stderr=subprocess.DEVNULL, timeout=60,
            )
    except Exception:
        print(f"mutation {name}: config rejected (ok)", file=sys.stderr)
        continue
    try:
        cfg = json.loads(out.read_text())
    except Exception as e:
        print(f"mutation {name}: invalid json {e}", file=sys.stderr)
        failures += 1
        continue
    services = cfg.get("services") or {}
    bad = False
    reason = ""
    if name == "wildcard-host":
        for svc, sc in services.items():
            for p in sc.get("ports") or []:
                hip = p.get("host_ip") or ""
                if hip != "127.0.0.1":
                    bad = True
                    reason = f"{svc} host_ip={hip!r}"
    elif name == "fixed-host-port":
        for svc, sc in services.items():
            for p in sc.get("ports") or []:
                pub = p.get("published")
                if pub not in (None, "", 0, "0"):
                    bad = True
                    reason = f"{svc} published={pub!r}"
    elif name == "missing-enable-label":
        for svc, sc in services.items():
            labels = sc.get("labels") or {}
            if isinstance(labels, list):
                kv = {}
                for item in labels:
                    if isinstance(item, str) and "=" in item:
                        k, v = item.split("=", 1)
                        kv[k] = v
                labels = kv
            if (sc.get("ports") or []) and str(labels.get("stacklane.enable", "")).lower() not in ("true", "1"):
                bad = True
                reason = f"{svc} missing enable"
                break
    if not bad:
        print(f"mutation {name}: FAIL expected defect not detected ({expect_hint})", file=sys.stderr)
        failures += 1
    else:
        print(f"mutation {name}: defect detected ({reason}) ok", file=sys.stderr)

if failures:
    sys.exit(2)
print("mutation probes: all defects detected", file=sys.stderr)
sys.exit(0)
PY
}

validate_rendered() {
  local json_path="$1"
  local expect_instance="$2"
  local expect_project="$3"
  python3 - "$json_path" "$expect_instance" "$expect_project" <<'PY'
import json, re, sys

path, expect_instance, expect_project = sys.argv[1], sys.argv[2], sys.argv[3]
PROJECT_SLUG = "crosstalk"
with open(path, "r", encoding="utf-8") as f:
    cfg = json.load(f)

errors = []

def err(msg):
    errors.append(msg)

services = cfg.get("services") or {}
for required in ("server", "postgres"):
    if required not in services:
        err(f"missing service {required}")

name = cfg.get("name") or ""
if name and name != expect_project:
    err(f"compose name {name!r} != expected project {expect_project!r}")

volumes_top = cfg.get("volumes") or {}
named_required = (
    "crosstalk_pgdata",
    "crosstalk_go_mod_cache",
    "crosstalk_go_build_cache",
)
vol_keys = set(volumes_top.keys())
for req in named_required:
    if req not in vol_keys and not any(req in k for k in vol_keys):
        err(f"missing named volume {req}")

def labels_map(sc):
    labels = sc.get("labels") or {}
    if isinstance(labels, list):
        out = {}
        for item in labels:
            if isinstance(item, str) and "=" in item:
                k, v = item.split("=", 1)
                out[k] = v
        return out
    if isinstance(labels, dict):
        return {str(k): str(v) for k, v in labels.items()}
    return {}

def check_ports(svc_name, sc, expect_target):
    ports = sc.get("ports") or []
    if not ports:
        err(f"{svc_name}: no published ports")
        return
    for p in ports:
        if not isinstance(p, dict):
            err(f"{svc_name}: port entry not object")
            continue
        proto = str(p.get("protocol") or "tcp").lower()
        if proto != "tcp":
            err(f"{svc_name}: only tcp publish is in scope (got {proto})")
        hip = p.get("host_ip")
        if hip != "127.0.0.1":
            err(f"{svc_name}: host_ip must be 127.0.0.1")
        published = p.get("published")
        if published not in (None, "", 0, "0"):
            err(f"{svc_name}: published must be ephemeral empty/0")
        target = p.get("target")
        try:
            target_int = int(target)
        except (TypeError, ValueError):
            err(f"{svc_name}: target port must be numeric")
            continue
        if target_int != int(expect_target):
            err(f"{svc_name}: target port want {expect_target}")

def check_isolation(svc_name, sc):
    if (sc.get("network_mode") or "") == "host":
        err(f"{svc_name}: network_mode=host forbidden")
    pid = sc.get("pid")
    if pid is not None and str(pid).strip().lower() in ("host", "\"host\""):
        err(f"{svc_name}: pid=host forbidden")
    priv = sc.get("privileged")
    if priv is True or (isinstance(priv, str) and priv.strip().lower() in ("true", "1", "yes", "on")):
        err(f"{svc_name}: privileged=true forbidden")

def validate_publishing_services():
    """Apply the generic Stacklane contract to every currently publishing service."""
    required_labels = (
        "stacklane.enable", "stacklane.project", "stacklane.instance",
        "stacklane.endpoint", "stacklane.port",
    )
    for svc_name, sc in services.items():
        ports = sc.get("ports") or []
        if not ports:
            continue
        check_isolation(svc_name, sc)
        labels = labels_map(sc)
        for key in required_labels:
            if not str(labels.get(key, "")):
                err(f"{svc_name}: missing {key}")
        if str(labels.get("stacklane.enable", "")) not in ("true", "1"):
            err(f"{svc_name}: stacklane.enable must be exactly true or 1")
        if str(labels.get("stacklane.project", "")) != PROJECT_SLUG:
            err(f"{svc_name}: stacklane.project must be {PROJECT_SLUG}")
        if str(labels.get("stacklane.instance", "")) != expect_instance:
            err(f"{svc_name}: stacklane.instance must match lifecycle instance")
        endpoint = str(labels.get("stacklane.endpoint", ""))
        if not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?", endpoint):
            err(f"{svc_name}: stacklane.endpoint must be a DNS slug")
        try:
            public_port = int(str(labels.get("stacklane.port", "")))
            if not 1 <= public_port <= 65535:
                raise ValueError
        except ValueError:
            err(f"{svc_name}: stacklane.port must be a valid numeric public port")
            public_port = None

        targets = set()
        for port in ports:
            if not isinstance(port, dict):
                err(f"{svc_name}: port entry not object")
                continue
            if str(port.get("protocol") or "tcp").lower() != "tcp":
                err(f"{svc_name}: only tcp publish is in scope")
            if port.get("host_ip") != "127.0.0.1":
                err(f"{svc_name}: host_ip must be 127.0.0.1")
            if port.get("published") not in (None, "", 0, "0"):
                err(f"{svc_name}: published port must be ephemeral")
            try:
                targets.add(int(port.get("target")))
            except (TypeError, ValueError):
                err(f"{svc_name}: target port must be numeric")
        if len(targets) != 1:
            err(f"{svc_name}: exactly one published target port is required")
            continue
        target = next(iter(targets))
        target_label = labels.get("stacklane.target_port")
        if target_label not in (None, ""):
            try:
                if int(str(target_label)) != target:
                    err(f"{svc_name}: stacklane.target_port must match published target")
            except ValueError:
                err(f"{svc_name}: stacklane.target_port must be numeric")
        elif public_port is not None and public_port != target:
            err(f"{svc_name}: stacklane.target_port required when public and target ports differ")
        if not (sc.get("healthcheck") or {}).get("test"):
            err(f"{svc_name}: publishing service requires a healthcheck")

validate_publishing_services()

def volume_entries(sc):
    return sc.get("volumes") or []

def has_bind(sc, target_suffix):
    for v in volume_entries(sc):
        if isinstance(v, dict):
            tgt = v.get("target") or v.get("destination") or ""
            typ = (v.get("type") or "").lower()
            if typ == "bind" and (tgt == target_suffix or tgt.endswith(target_suffix)):
                return True
        elif isinstance(v, str):
            if v.rstrip("/").endswith(":" + target_suffix.rstrip("/")) or f":{target_suffix}" in v:
                return True
    return False

def has_named(sc, name_part):
    for v in volume_entries(sc):
        if isinstance(v, dict):
            src = str(v.get("source") or "")
            typ = (v.get("type") or "").lower()
            if typ == "volume" and name_part in src:
                return True
            if name_part in src:
                return True
        elif isinstance(v, str) and name_part in v:
            return True
    return False

def as_env(raw):
    if isinstance(raw, list):
        out = {}
        for e in raw:
            if isinstance(e, str):
                k, _, v = e.partition("=")
                out[k] = v
        return out
    if isinstance(raw, dict):
        return {str(k): str(v) for k, v in raw.items()}
    return {}

server = services.get("server") or {}
check_ports("server", server, 8080)
check_isolation("server", server)
al = labels_map(server)
for k, want in {
    "stacklane.enable": "true",
    "stacklane.project": "crosstalk",
    "stacklane.instance": expect_instance,
    "stacklane.endpoint": "api",
    "stacklane.port": "8080",
}.items():
    got = str(al.get(k, ""))
    if got != want:
        err(f"server label {k} mismatch")

if str(al.get("stacklane.enable", "")) not in ("true", "1"):
    err("server stacklane.enable must be true or 1")

if not has_bind(server, "/app/server"):
    err("server: missing source bind mount to /app/server")
if not has_bind(server, "/app/proto"):
    err("server: missing source bind mount to /app/proto")
for nv in ("crosstalk_go_mod_cache", "crosstalk_go_build_cache"):
    if not has_named(server, nv):
        err(f"server: missing named volume involving {nv}")

env = as_env(server.get("environment") or {})
if env.get("CT_ADDR") != ":8080":
    err("server CT_ADDR must be :8080")
dsn = env.get("CT_DATABASE_URL", "")
if "postgres:5432" not in dsn:
    err("server CT_DATABASE_URL must use Compose DNS postgres:5432")
if "localhost" in dsn or "127.0.0.1" in dsn:
    err("server CT_DATABASE_URL must not use host loopback")

hc = server.get("healthcheck") or {}
test = hc.get("test") or []
test_s = test if isinstance(test, str) else " ".join(str(x) for x in test)
if "/api/openapi.json" not in test_s:
    err("server healthcheck must hit /api/openapi.json")

postgres = services.get("postgres") or {}
check_ports("postgres", postgres, 5432)
check_isolation("postgres", postgres)
pl = labels_map(postgres)
for k, want in {
    "stacklane.enable": "true",
    "stacklane.project": "crosstalk",
    "stacklane.instance": expect_instance,
    "stacklane.endpoint": "postgres",
    "stacklane.port": "5432",
}.items():
    got = str(pl.get(k, ""))
    if got != want:
        err(f"postgres label {k} mismatch")

if not has_named(postgres, "crosstalk_pgdata"):
    err("postgres: missing named volume involving crosstalk_pgdata")

phc = postgres.get("healthcheck") or {}
ptest = phc.get("test") or []
ptest_s = ptest if isinstance(ptest, str) else " ".join(str(x) for x in ptest)
if "pg_isready" not in ptest_s:
    err("postgres healthcheck must use pg_isready")

deps = server.get("depends_on") or {}
if isinstance(deps, dict):
    pg_dep = deps.get("postgres") or {}
    cond = pg_dep.get("condition") if isinstance(pg_dep, dict) else None
    if cond and cond != "service_healthy":
        err("server depends_on.postgres.condition want service_healthy")
elif isinstance(deps, list) and "postgres" not in deps:
    err("server depends_on missing postgres")

blob = json.dumps({"server": env, "labels": [al, pl]})
if ".local" in blob.lower():
    err("compose-derived config must not use .local domains")

if errors:
    for e in errors:
        print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
print(f"validated instance={expect_instance} project={expect_project}", file=sys.stderr)
sys.exit(0)
PY
}

main() {
  require_tools
  local instance project
  instance="$(derive_instance)"
  project="${PROJECT_SLUG}-${instance}"
  export STACKLANE_INSTANCE="$instance"
  export STACKLANE_BASE_DOMAIN="${STACKLANE_BASE_DOMAIN:-test}"

  umask 077
  COMPOSE_CHECK_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/crosstalk-compose-check.XXXXXX")"
  chmod 700 "$COMPOSE_CHECK_TMPDIR"
  # shellcheck disable=SC2064
  trap 'rm -rf "${COMPOSE_CHECK_TMPDIR:-}"' EXIT INT TERM

  local cfg1 cfg2
  cfg1="${COMPOSE_CHECK_TMPDIR}/compose-${instance}.json"
  info "rendering compose config for instance=${instance} project=${project}"
  render_config "$project" "$instance" "$cfg1"
  validate_rendered "$cfg1" "$instance" "$project"
  ok "default instance path (${instance})"

  local alt="slc-altcheck"
  local alt_project="${PROJECT_SLUG}-${alt}"
  cfg2="${COMPOSE_CHECK_TMPDIR}/compose-${alt}.json"
  STACKLANE_INSTANCE="$alt" render_config "$alt_project" "$alt" "$cfg2"
  validate_rendered "$cfg2" "$alt" "$alt_project"
  if cmp -s "$cfg1" "$cfg2"; then
    die "two instances produced identical rendered configs"
  fi
  ok "override instance path differs (${alt})"

  info "running mutation probes (fail-closed)"
  run_mutation_probes "$COMPOSE_CHECK_TMPDIR"
  ok "mutation probes"

  ok "all compose-dev checks passed"
}

main "$@"
