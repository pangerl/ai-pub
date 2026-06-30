#!/usr/bin/env python3
"""Small AI Pub Agent API client for the ai-pub-release skill."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid


SKILL_VERSION = "0.1.0"


def main() -> int:
    parser = argparse.ArgumentParser(description="Call AI Pub Agent API")
    sub = parser.add_subparsers(dest="command", required=True)

    services = sub.add_parser("services", help="List service candidates")
    services.add_argument("--query", "-q", default="")
    services.add_argument("--project-id", default="")

    environments = sub.add_parser("environments", help="List environment candidates")
    environments.add_argument("--query", "-q", default="")

    versions = sub.add_parser("versions", help="List service version candidates")
    versions.add_argument("--service-id", required=True)
    versions.add_argument("--query", "-q", default="")

    targets = sub.add_parser("targets", help="List deployment target candidates")
    targets.add_argument("--service-id", required=True)
    targets.add_argument("--environment-id", required=True)

    preflight = sub.add_parser("preflight", help="Run release intent preflight")
    add_release_fields(preflight)

    create = sub.add_parser("create", help="Create an AI Agent release request")
    add_release_fields(create)
    create.add_argument("--idempotency-key", default="")
    create.add_argument("--agent-name", default="codex")
    create.add_argument("--skill-version", default=SKILL_VERSION)
    create.add_argument("--intent-summary", default="")
    create.add_argument("--client-request-id", default="")
    create.add_argument("--conversation-ref", default="")

    confirm = sub.add_parser("confirm", help="Confirm a release request")
    confirm.add_argument("--release-id", required=True)

    summary = sub.add_parser("summary", help="Read release summary")
    summary.add_argument("--release-id", required=True)

    args = parser.parse_args()
    client = Client.from_env()

    if args.command == "services":
        params = {"q": args.query, "project_id": args.project_id}
        return emit(client.get("/api/v1/agent/services", params))
    if args.command == "environments":
        return emit(client.get("/api/v1/agent/environments", {"q": args.query}))
    if args.command == "versions":
        path = f"/api/v1/agent/services/{quote_path(args.service_id)}/versions"
        return emit(client.get(path, {"q": args.query}))
    if args.command == "targets":
        return emit(
            client.get(
                "/api/v1/agent/deployment-targets",
                {"service_id": args.service_id, "environment_id": args.environment_id},
            )
        )
    if args.command == "preflight":
        return emit(client.post("/api/v1/agent/release-intents/preflight", release_payload(args)))
    if args.command == "create":
        payload = release_payload(args)
        payload.update(
            {
                "idempotency_key": args.idempotency_key or f"agent:{uuid.uuid4()}",
                "agent_name": args.agent_name,
                "skill_version": args.skill_version,
                "intent_summary": args.intent_summary,
                "client_request_id": args.client_request_id,
                "conversation_ref": args.conversation_ref,
            }
        )
        return emit(client.post("/api/v1/agent/release-requests", payload))
    if args.command == "confirm":
        path = f"/api/v1/agent/release-requests/{quote_path(args.release_id)}/confirm"
        return emit(client.post(path, {}))
    if args.command == "summary":
        path = f"/api/v1/agent/release-requests/{quote_path(args.release_id)}/summary"
        return emit(client.get(path, {}))
    raise AssertionError(args.command)


def add_release_fields(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--service-id", required=True)
    parser.add_argument("--environment-id", required=True)
    parser.add_argument("--version-id", required=True)
    parser.add_argument("--target-id", required=True)


def release_payload(args: argparse.Namespace) -> dict[str, str]:
    return {
        "service_id": args.service_id,
        "environment_id": args.environment_id,
        "service_version_id": args.version_id,
        "deployment_target_id": args.target_id,
    }


def emit(payload: object) -> int:
    print(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


def quote_path(value: str) -> str:
    return urllib.parse.quote(value, safe="")


class Client:
    def __init__(self, base_url: str, api_key: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key

    @classmethod
    def from_env(cls) -> "Client":
        base_url = os.environ.get("AI_PUB_BASE_URL", "").strip()
        api_key = os.environ.get("AI_PUB_API_KEY", "").strip()
        if not base_url:
            die("AI_PUB_BASE_URL is required")
        if not api_key:
            die("AI_PUB_API_KEY is required")
        return cls(base_url, api_key)

    def get(self, path: str, params: dict[str, str]) -> object:
        clean = {k: v for k, v in params.items() if v}
        query = urllib.parse.urlencode(clean)
        url = self.base_url + path + (f"?{query}" if query else "")
        return self.request("GET", url, None)

    def post(self, path: str, payload: dict[str, object]) -> object:
        return self.request("POST", self.base_url + path, payload)

    def request(self, method: str, url: str, payload: dict[str, object] | None) -> object:
        data = None
        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {self.api_key}",
        }
        if payload is not None:
            data = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as err:
            body = err.read().decode("utf-8", "replace")
            try:
                parsed = json.loads(body)
            except json.JSONDecodeError:
                parsed = {"error": {"code": "http_error", "message": body}}
            print(json.dumps(parsed, ensure_ascii=False, indent=2, sort_keys=True), file=sys.stderr)
            return_code = err.code if 1 <= err.code <= 255 else 1
            raise SystemExit(return_code)
        except urllib.error.URLError as err:
            die(f"request failed: {err}")


def die(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


if __name__ == "__main__":
    raise SystemExit(main())
