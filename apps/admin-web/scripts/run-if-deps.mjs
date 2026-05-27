import { existsSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const [command, ...args] = process.argv.slice(2);

if (!command) {
  console.error("No command provided.");
  process.exit(1);
}

const binary = join(process.cwd(), "node_modules", ".bin", command);

if (!existsSync(binary)) {
  console.log(`skip ${command}: dependencies are not installed; run npm install or pnpm install in this app`);
  process.exit(0);
}

const result = spawnSync(binary, args, { stdio: "inherit" });

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

process.exit(result.status ?? 1);
