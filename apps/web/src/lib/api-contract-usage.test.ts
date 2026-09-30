import fs from "node:fs";
import path from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

type OpenAPISpec = {
  paths: Record<string, unknown>;
};

const repoRoot = path.resolve(__dirname, "../../../..");
const srcRoot = path.resolve(__dirname, "..");
const openapiPath = path.join(repoRoot, "docs/openapi/openapi.json");

const legacyAliases: Record<string, string> = {
  "/api/public/modes": "/api/public/demo-modes",
};

const frontendSubroutes = new Set([
  "/api/admin/modes/{id}/copy",
  "/api/admin/modes/{id}/detach-paid-tariffs",
  "/api/admin/modes/{id}/test-model",
  // Prompt version history/rollback: suffix routes under /api/admin/modes/
  // (see openapi_contract_test.go: x-router-path must be unique,
  // so these suffixes are not documented separately, like copy/test-model/
  // detach-paid-tariffs above). {sha} is normalised to {id} by the same logic
  // as the numeric mode id, hence the double {id} in the paths below.
  "/api/admin/modes/{id}/history",
  "/api/admin/modes/{id}/history/{id}",
  "/api/admin/modes/{id}/restore/{id}",
  // Bulk AI change for all modes of a tariff: a suffix route under /api/admin/tariffs/
  "/api/admin/tariffs/{id}/set-modes-ai",
  "/api/admin/users/{id}/access",
]);

const ignoredLiterals = new Set([
  "/api/_error",
]);

function walkFiles(dir: string): string[] {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  return entries.flatMap((entry) => {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (["generated", ".next", "node_modules"].includes(entry.name)) return [];
      return walkFiles(fullPath);
    }
    if (!entry.isFile()) return [];
    if (!/\.(ts|tsx)$/.test(entry.name)) return [];
    if (/\.(test|spec)\.(ts|tsx)$/.test(entry.name)) return [];
    return [fullPath];
  });
}

function apiLiteralsFrom(source: string): string[] {
  const literals = new Set<string>();
  const sourceFile = ts.createSourceFile("scan.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const visit = (node: ts.Node) => {
    let value = "";
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      value = node.text;
    } else if (ts.isTemplateExpression(node)) {
      value = node.getText(sourceFile).slice(1, -1);
    }
    if (value.includes("/api/")) {
      const paths = value.match(/\/api\/[A-Za-z0-9_.$?=&%{}[\]/:-]+/g) || [];
      for (const rawPath of paths) {
        literals.add(rawPath);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return [...literals];
}

function normalizeApiLiteral(rawPath: string): string {
  if (rawPath.startsWith("/api/auth/oauth/${")) {
    return "/api/auth/oauth/{provider}/{action}";
  }
  let normalized = rawPath
    .replace(/\\`/g, "`")
    .replace(/\$\{[^}]+}/g, "{id}")
    .replace(/\/\{id}\([^)]+\)/g, "/{id}")
    .replace(/\?.*$/, "")
    .replace(/[.,;:)]+$/, "")
    .replace(/\/+$/, "");

  normalized = normalized.replace(/\/\{id}\/callback$/, "/{provider}/{action}");
  normalized = normalized.replace(/\/yandex\/(start|callback)$/, "/{provider}/{action}");
  normalized = normalized.replace(/\/\{id}\/(start|callback)$/, "/{provider}/{action}");
  normalized = normalized.replace(/\/\{id}$/, "/{id}");
  return normalized;
}

describe("frontend API usage contract", () => {
  it("uses OpenAPI paths or explicit frontend aliases", () => {
    const spec = JSON.parse(fs.readFileSync(openapiPath, "utf8")) as OpenAPISpec;
    const openapiPaths = new Set(Object.keys(spec.paths));

    const unknown: string[] = [];
    for (const file of walkFiles(srcRoot)) {
      const rel = path.relative(repoRoot, file);
      for (const rawPath of apiLiteralsFrom(fs.readFileSync(file, "utf8"))) {
        const normalized = normalizeApiLiteral(rawPath);
        const canonical = legacyAliases[normalized] || normalized;
        if (ignoredLiterals.has(normalized) || openapiPaths.has(canonical) || frontendSubroutes.has(normalized)) {
          continue;
        }
        unknown.push(`${rel}: ${rawPath} -> ${normalized}`);
      }
    }

    expect(unknown).toEqual([]);
  });

  it("keeps legacy aliases mapped to real OpenAPI paths", () => {
    const spec = JSON.parse(fs.readFileSync(openapiPath, "utf8")) as OpenAPISpec;
    for (const [alias, canonical] of Object.entries(legacyAliases)) {
      expect(alias).not.toBe(canonical);
      expect(spec.paths[canonical], `${alias} must point to documented ${canonical}`).toBeTruthy();
    }
  });
});
