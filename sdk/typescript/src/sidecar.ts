import { spawn, type ChildProcess } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import * as http from 'http';

export interface SidecarOptions {
  name: string;
  command: string[];
  healthCheck?: string;
  startOn?: 'server_start' | 'first_tool_call';
  healthTimeout?: number;
}

interface SidecarDef {
  name: string;
  command: string[];
  healthCheck: string;
  startOn: string;
  healthTimeout: number;
}

const sidecarRegistry: SidecarDef[] = [];
const runningProcesses: Map<string, ChildProcess> = new Map();

export function sidecar(options: SidecarOptions): void {
  sidecarRegistry.push({
    name: options.name,
    command: options.command,
    healthCheck: options.healthCheck ?? '',
    startOn: options.startOn ?? 'first_tool_call',
    healthTimeout: options.healthTimeout ?? 30,
  });
}

export function getRegisteredSidecars(): SidecarDef[] {
  return [...sidecarRegistry];
}

export function clearSidecarRegistry(): void {
  sidecarRegistry.length = 0;
}

function pidFilePath(name: string): string {
  return path.join(os.homedir(), '.protomcp', 'sidecars', `${name}.pid`);
}

function checkHealth(sc: SidecarDef): Promise<boolean> {
  if (!sc.healthCheck) return Promise.resolve(true);
  return new Promise((resolve) => {
    const req = http.get(sc.healthCheck, { timeout: 5000 }, (res) => {
      resolve(res.statusCode === 200);
      res.resume();
    });
    req.on('error', () => resolve(false));
    req.on('timeout', () => {
      req.destroy();
      resolve(false);
    });
  });
}

async function startSidecar(sc: SidecarDef): Promise<void> {
  const existing = runningProcesses.get(sc.name);
  if (existing && existing.exitCode === null) {
    const healthy = await checkHealth(sc);
    if (healthy) return;
  }

  const pidDir = path.dirname(pidFilePath(sc.name));
  fs.mkdirSync(pidDir, { recursive: true });

  const proc = spawn(sc.command[0], sc.command.slice(1), {
    detached: true,
    stdio: 'ignore',
  });
  proc.unref();
  runningProcesses.set(sc.name, proc);

  if (proc.pid !== undefined) {
    fs.writeFileSync(pidFilePath(sc.name), String(proc.pid));
  }

  if (sc.healthCheck) {
    const deadline = Date.now() + sc.healthTimeout * 1000;
    while (Date.now() < deadline) {
      const healthy = await checkHealth(sc);
      if (healthy) return;
      await new Promise(r => setTimeout(r, 1000));
    }
  }
}

function stopSidecar(sc: SidecarDef): void {
  const proc = runningProcesses.get(sc.name);
  runningProcesses.delete(sc.name);
  if (proc && proc.exitCode === null) {
    try {
      proc.kill('SIGTERM');
    } catch {
      // process already gone
    }
  }
  cleanupPidFile(sc.name);
}

function cleanupPidFile(name: string): void {
  try {
    fs.unlinkSync(pidFilePath(name));
  } catch {
    // ignore
  }
}

export async function startSidecars(trigger: string): Promise<void> {
  for (const sc of sidecarRegistry) {
    if (sc.startOn === trigger) {
      await startSidecar(sc);
    }
  }
}

export function stopAllSidecars(): void {
  for (const sc of sidecarRegistry) {
    stopSidecar(sc);
  }
}

// Cleanup on process exit
process.on('exit', () => stopAllSidecars());
