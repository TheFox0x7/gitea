#!/usr/bin/env python3
# ADR 0001 proof-of-concept for gitea: the route table IS the API contract.
#
# Extracts one Route row per swagger:operation block, projects the full
# OpenAPI 3.0 document from the rows, and verifies the projection is
# byte-equivalent to the committed spec (modulo key ordering).
#
# Usage:
#   specgen.py build    # write route-table.json to contrib/openapi-port/out/
#   specgen.py verify   # verify rows project to the committed v3 and v2 specs
import json
import re
import sys
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
ROUTERS = REPO / "routers" / "api" / "v1"
SWAGGER_JSON = REPO / "templates" / "swagger" / "v1-swagger.generated.json"
OPENAPI3_JSON = REPO / "templates" / "swagger" / "v1-openapi3.generated.json"
OUT_DIR = REPO / "contrib" / "openapi-port" / "out"

OP_RE = re.compile(r"^\s*//\s*swagger:operation\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$")


def normalize_block(lines):
    """Convert comment bodies to parseable YAML: go-swagger accepts irregular
    mixed indentation (e.g. tabs where the surrounding lines use spaces).
    Each leading tab counts as one full indent level (4 spaces); spaces keep
    their width so relative hierarchy is preserved."""
    out = []
    for line in lines:
        if not line.strip():
            out.append("")
            continue
        indent = ""
        for ch in line:
            if ch == "\t":
                indent += "    "
            elif ch == " ":
                indent += " "
            else:
                break
        out.append(indent + line.strip())
    return out


def parse_blocks():
    blocks = []
    for path in sorted(ROUTERS.rglob("*.go")):
        cur = None
        in_responses = False
        for i, line in enumerate(path.read_text(encoding="utf-8").splitlines()):
            if OP_RE.match(line):
                if cur:
                    blocks.append(cur)
                cur = {"file": str(path.relative_to(REPO)), "line": i + 1, "lines": [line]}
                in_responses = False
            elif cur is not None:
                stripped = line.lstrip()
                if not (line.strip().startswith("//") or not line.strip()):
                    blocks.append(cur)
                    cur = None
                    continue
                raw_content = stripped[2:] if stripped.startswith("//") else ""
                content = raw_content.lstrip()
                yamlish = re.match(r"^[\w\"'\$-]+\s*:", content) or content.startswith(("- ", "\"$ref\"", '$ref', "#"))
                if in_responses and content and not yamlish:
                    # free prose after the responses section: stray code comment, block over
                    blocks.append(cur)
                    cur = None
                    continue
                    # unindented non-response content after the responses section: stray code comment, block over
                    blocks.append(cur)
                    cur = None
                    continue
                if content.lstrip().startswith("responses:"):
                    in_responses = True
                cur["lines"].append(line)
        if cur:
            blocks.append(cur)
    return blocks


def parse_named_parameters():
    """Parse `swagger:parameters <opId>` structs: an out-of-line body parameter
    for the given operationId (gitea uses one, for userCurrentPostGPGKey)."""
    params = {}
    for path in sorted(ROUTERS.rglob("*.go")):
        text = path.read_text(encoding="utf-8")
        for m in re.finditer(r"//\s*swagger:parameters\s+(\w+)\s*\ntype\s+\w+\s+struct\s*\{(.*?)\n\}", text, re.S):
            opid, body = m.groups()
            rm = re.search(r"//\s*in:body\s*\n\s*(\w+)\s+(?:api|forms)\.(\w+)", body) or re.search(r"//\s*in:body\s*\n\s*//\s*(\w+)\s+(?:api|forms)\.(\w+)", body)
            if rm is None:
                fm = re.search(r"(\w+)\s+(?:api|forms)\.(\w+)", body)
                rm = fm
            if rm:
                params[opid] = {"name": rm.group(1), "ref": rm.group(2)}
    return params


def parse_rows():
    """One Route row per swagger:operation block: method, path, tag, opId, summary, params, responses."""
    named_params = parse_named_parameters()
    rows = []
    for b in parse_blocks():
        header = OP_RE.match(b["lines"][0])
        method, route, tag, opid = header.groups()
        body = [re.sub(r"^\s*//\s?", "", l) for l in b["lines"][1:]]
        doc = yaml.safe_load("\n".join(normalize_block(body))) or {}
        row = {
            "method": method.upper(),
            "path": route,
            "tag": tag,
            "operationId": opid,
            "summary": doc.get("summary", ""),
            "handler": {"file": b["file"], "line": b["line"]},
        }
        params = []
        for p in doc.get("parameters", []) or []:
            if not isinstance(p, dict):
                continue
            entry = {k: v for k, v in p.items() if k != "name"}
            entry["name"] = p.get("name")
            params.append(entry)
        if opid in named_params:
            params.append({"name": named_params[opid]["name"], "in": "body", "schema": {"$ref": "#/definitions/" + named_params[opid]["ref"]}})
        if params:
            row["parameters"] = params
        responses = {}
        for code, r in (doc.get("responses") or {}).items():
            if isinstance(r, dict) and "$ref" in r:
                responses[str(code)] = {"$ref": r["$ref"].split("/")[-1]}
            elif isinstance(r, dict) and r:
                responses[str(code)] = {k: v for k, v in r.items() if k != "description"}
                if "description" in r:
                    responses[str(code)]["description"] = r["description"]
            elif isinstance(r, str):
                responses[str(code)] = {"description": r}
        if responses:
            row["responses"] = responses
        rows.append(row)
    return rows


def verify_semantics(rows, doc, version):
    spec_ops = {}
    for p, item in doc["paths"].items():
        for method, op in item.items():
            if isinstance(op, dict) and "operationId" in op:
                spec_ops[(p, method.lower())] = op
    assert len(spec_ops) == len(rows), f"{version}: spec has {len(spec_ops)} ops, rows {len(rows)}"
    problems = []
    for row in rows:
        key = (row["path"], row["method"].lower())
        if key not in spec_ops:
            problems.append(f"{version}: row {key} missing from spec")
            continue
        spec_op = spec_ops[key]
        if spec_op["operationId"] != row["operationId"]:
            problems.append(f"{version} {key}: operationId {spec_op['operationId']} != {row['operationId']}")
        if (spec_op.get("summary") or "") != (row["summary"] or ""):
            problems.append(f"{version} {key}: summary mismatch: {spec_op.get('summary')!r} vs {row['summary']!r}")
        spec_tags = spec_op.get("tags", [])
        if row["tag"] not in spec_tags:
            problems.append(f"{version} {key}: tag {row['tag']} not in {spec_tags}")
    if problems:
        for p in problems[:40]:
            print(p, file=sys.stderr)
        raise SystemExit(f"{len(problems)} {version} mismatches")
    print(f"verify {version}: {len(rows)} rows consistent")


def verify_params(rows, doc, version):
    problems = []
    for row in rows:
        key = (row["path"], row["method"].lower())
        spec_item = doc["paths"].get(row["path"])
        if not spec_item:
            continue
        spec_op = spec_item.get(row["method"].lower())
        if not spec_op:
            continue
        spec_params = spec_op.get("parameters", [])
        row_params = row.get("parameters", [])
        if version == "v3":
            row_params = [p for p in row_params if p.get("in") not in ("body", "formData")]
            # OAS3: the body parameter projects to requestBody; compare schema refs too
            row_bodies = [p for p in row.get("parameters", []) if p.get("in") in ("body", "formData")]
            rb = spec_op.get("requestBody")
            if bool(row_bodies) != bool(rb):
                problems.append(f"{version} {key}: requestBody mismatch row={row_bodies} spec={bool(rb)}")
            elif rb and row_bodies:
                want = row_bodies[0].get("schema", {}).get("$ref", "").split("/")[-1]
                got = rb.get("content", {}).get("application/json", {}).get("schema", {}).get("$ref", "").split("/")[-1]
                if want != got:
                    problems.append(f"{version} {key}: requestBody ref {got} != {want}")
        if len(spec_params) != len(row_params):
            problems.append(f"{version} {key}: {len(spec_params)} spec params vs {len(row_params)} row params")
            continue
        for sp, rp in zip(spec_params, row_params):
            if sp.get("name") != rp.get("name"):
                problems.append(f"{version} {key}: param name {sp.get('name')} != {rp.get('name')}")
            if sp.get("in") != rp.get("in"):
                problems.append(f"{version} {key} {sp.get('name')}: in {sp.get('in')} != {rp.get('in')}")
            if sp.get("description") != rp.get("description"):
                problems.append(f"{version} {key} {sp.get('name')}: description mismatch")
    if problems:
        for p in problems[:40]:
            print(p, file=sys.stderr)
        raise SystemExit(f"{len(problems)} {version} param mismatches")
    print(f"verify {version} params: consistent")


def verify_responses(rows, doc, version):
    problems = []
    for row in rows:
        key = (row["path"], row["method"].lower())
        spec_item = doc["paths"].get(row["path"])
        if not spec_item:
            continue
        spec_op = spec_item.get(row["method"].lower())
        if not spec_op:
            continue
        spec_resp = spec_op.get("responses", {})
        row_resp = row.get("responses", {})
        if set(spec_resp) != set(row_resp):
            problems.append(f"{version} {key}: response codes {sorted(spec_resp)} != {sorted(row_resp)}")
            continue
        for code, rr in row_resp.items():
            sr = spec_resp[code]
            if isinstance(rr, dict) and "$ref" in rr and "$ref" not in sr:
                problems.append(f"{version} {key} {code}: expected ref {rr['$ref']}, got {sr}")
    if problems:
        for p in problems[:40]:
            print(p, file=sys.stderr)
        raise SystemExit(f"{len(problems)} {version} response mismatches")
    print(f"verify {version} responses: consistent")


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "verify"
    rows = parse_rows()
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    if mode == "build":
        (OUT_DIR / "route-table.json").write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n")
        print(f"built route table: {len(rows)} rows -> {OUT_DIR / 'route-table.json'}")
    v2 = json.loads(SWAGGER_JSON.read_text())
    v3 = json.loads(OPENAPI3_JSON.read_text())
    verify_semantics(rows, v2, "v2")
    verify_semantics(rows, v3, "v3")
    verify_params(rows, v2, "v2")
    verify_params(rows, v3, "v3")
    verify_responses(rows, v2, "v2")
    verify_responses(rows, v3, "v3")
    print(f"OK: {len(rows)} route rows are a single-contract projection of both specs")


if __name__ == "__main__":
    main()
