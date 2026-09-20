import type { Plugin } from "@opencode-ai/plugin"
import { tool } from "@opencode-ai/plugin"
import { execFile } from "node:child_process"
import { promisify } from "node:util"
import { homedir } from "node:os"
import { delimiter, join } from "node:path"
import { existsSync } from "node:fs"
import { fileURLToPath } from "node:url"

const INSTALL_DIR = join(process.env.REMOTE_SHELL_PREFIX || join(homedir(), ".remote-shell"), "bin")
const execFileAsync = promisify(execFile)

function cli(name: string) {
  return join(INSTALL_DIR, process.platform === "win32" ? `${name}.exe` : name)
}

async function run(bin: string, args: string[], timeoutMs: number) {
  const { stdout, stderr } = await execFileAsync(bin, args, {
    timeout: timeoutMs,
    maxBuffer: 64 * 1024 * 1024,
  })
  return (stdout || "") + (stderr ? `\n[stderr]\n${stderr}` : "")
}

export const RemoteShell: Plugin = async () => {
  // Ensure prebuilt binaries exist (prebuilt download; no Go toolchain needed).
  const bootName = process.platform === "win32" ? "bootstrap.ps1" : "bootstrap.sh"
  const sidecar = fileURLToPath(new URL(`./remote-shell-scripts/${bootName}`, import.meta.url))
  const bootScript = existsSync(sidecar) ? sidecar : fileURLToPath(new URL(`./scripts/${bootName}`, import.meta.url))
  try {
    await execFileAsync(process.platform === "win32" ? "powershell.exe" : "bash",
      process.platform === "win32" ? ["-NoProfile", "-File", bootScript] : [bootScript])
  } catch {
    // Binary may already be present; the shell.env hook and tools fall back
    // to absolute paths and surface real errors if truly missing.
  }

  return {
    "shell.env": async (_input, output) => {
      output.env.PATH = [INSTALL_DIR, output.env.PATH].filter(Boolean).join(delimiter)
    },

    tool: {
      start_remote_shell: tool({
        description:
          "Start the remote-shell daemon for a named connection (from ~/.remote-shell/config.toml) so commands can be run against it. No-op if already running.",
        args: {
          conn: tool.schema.string().describe("Named connection defined in ~/.remote-shell/config.toml"),
        },
        async execute(args) {
          try {
            return { output: await run(cli("start-remote-shell"), ["-conn", args.conn], 90_000) }
          } catch (err: any) {
            if (err.code === 1 && err.stdout) return { output: err.stdout + (err.stderr ? `\n[stderr]\n${err.stderr}` : "") }
            return { output: `start-remote-shell failed: ${err.message}` }
          }
        },
      }),

      remote_shell: tool({
        description:
          "Run a command on a remote machine over SSH through the running remote-shell daemon. Pass the command as a single -c expression (required for Windows remotes and for shell features like pipes/globs). For binary output (e.g. screenshots) prefer the bash tool instead.",
        args: {
          conn: tool.schema.string().describe("Named connection defined in ~/.remote-shell/config.toml"),
          command: tool.schema.string().describe("Shell expression to run on the remote (one -c argument)"),
        },
        async execute(args) {
          try {
            return { output: await run(cli("remote-shell"), ["-conn", args.conn, "-c", args.command], 300_000) }
          } catch (err: any) {
            if (err.code !== undefined && err.stdout) {
              return {
                output: (err.stdout || "") + (err.stderr ? `\n[stderr]\n${err.stderr}` : ""),
                metadata: { exitCode: err.code },
              }
            }
            return { output: `remote-shell failed: ${err.message}` }
          }
        },
      }),

      remote_shell_info: tool({
        description:
          "Show connection status and detected remote OS/shell metadata (os, architecture, os_version, hostname, default_shell) for one connection or all.",
        args: {
          conn: tool.schema.string().optional().describe("Connection name; omit to list all"),
        },
        async execute(args) {
          try {
            const argv = args.conn ? ["-conn", args.conn, "-json"] : ["--all", "-json"]
            return { output: await run(cli("remote-shell-info"), argv, 60_000) }
          } catch (err: any) {
            return { output: `remote-shell-info failed: ${err.message}` }
          }
        },
      }),

      stop_remote_shell: tool({
        description: "Stop the remote-shell daemon for one connection or all connections.",
        args: {
          conn: tool.schema.string().optional().describe("Connection name; omit to stop all"),
        },
        async execute(args) {
          try {
            const argv = args.conn ? ["-conn", args.conn] : ["--all"]
            return { output: await run(cli("stop-remote-shell"), argv, 60_000) }
          } catch (err: any) {
            return { output: `stop-remote-shell failed: ${err.message}` }
          }
        },
      }),
    },
  }
}
