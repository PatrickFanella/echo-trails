import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const root = resolve("src");
const forbidden = /["'`]\/(?:admin|account|analytics|appeals|auth|beta|blocks|consent|creation-tokens|devices|discovery|echoes|media|mutes|notification|places|privacy|replies|reports|search|sessions|subscriptions|users)(?:[/?"'`$])/;

function files(directory) {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return entry === "generated" ? [] : files(path);
    return /\.(ts|tsx)$/.test(entry) && !/\.test\.(ts|tsx)$/.test(entry) ? [path] : [];
  });
}

for (const path of files(root)) {
  const match = forbidden.exec(readFileSync(path, "utf8"));
  if (match) {
    console.error(`${relative(process.cwd(), path)} contains handwritten API path ${match[0]}; use generated openApiOperations`);
    process.exit(1);
  }
}

console.log("admin OpenAPI path boundary check passed");
