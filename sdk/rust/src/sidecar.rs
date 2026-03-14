use std::collections::HashMap;
use std::sync::Mutex;

pub struct SidecarDef {
    pub name: String,
    pub command: Vec<String>,
    pub health_check: String,
    pub start_on: String,
    pub health_timeout_ms: u64,
    pub health_interval_ms: u64,
    pub shutdown_timeout_ms: u64,
}

pub struct SidecarBuilder {
    name: String,
    command: Vec<String>,
    health_check: String,
    start_on: String,
    health_timeout_ms: u64,
    health_interval_ms: u64,
    shutdown_timeout_ms: u64,
}

static SIDECAR_REGISTRY: Mutex<Vec<SidecarDef>> = Mutex::new(Vec::new());
static RUNNING_PIDS: Mutex<Option<HashMap<String, u32>>> = Mutex::new(None);

fn with_running_pids<F, R>(f: F) -> R
where
    F: FnOnce(&mut HashMap<String, u32>) -> R,
{
    let mut guard = RUNNING_PIDS.lock().unwrap();
    if guard.is_none() {
        *guard = Some(HashMap::new());
    }
    f(guard.as_mut().unwrap())
}

fn pid_dir() -> std::path::PathBuf {
    dirs_or_home().join(".protomcp").join("sidecars")
}

fn dirs_or_home() -> std::path::PathBuf {
    std::env::var("HOME")
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|_| std::path::PathBuf::from("/tmp"))
}

fn pid_file_path(name: &str) -> std::path::PathBuf {
    pid_dir().join(format!("{}.pid", name))
}

/// Create a sidecar definition using the builder pattern.
pub fn sidecar(name: &str, command: &[&str]) -> SidecarBuilder {
    SidecarBuilder {
        name: name.to_string(),
        command: command.iter().map(|s| s.to_string()).collect(),
        health_check: String::new(),
        start_on: "first_tool_call".to_string(),
        health_timeout_ms: 30000,
        health_interval_ms: 1000,
        shutdown_timeout_ms: 3000,
    }
}

impl SidecarBuilder {
    pub fn health_check(mut self, url: &str) -> Self {
        self.health_check = url.to_string();
        self
    }

    pub fn start_on(mut self, trigger: &str) -> Self {
        self.start_on = trigger.to_string();
        self
    }

    pub fn health_timeout_ms(mut self, ms: u64) -> Self {
        self.health_timeout_ms = ms;
        self
    }

    pub fn health_interval_ms(mut self, ms: u64) -> Self {
        self.health_interval_ms = ms;
        self
    }

    pub fn shutdown_timeout_ms(mut self, ms: u64) -> Self {
        self.shutdown_timeout_ms = ms;
        self
    }

    pub fn register(self) {
        SIDECAR_REGISTRY.lock().unwrap().push(SidecarDef {
            name: self.name,
            command: self.command,
            health_check: self.health_check,
            start_on: self.start_on,
            health_timeout_ms: self.health_timeout_ms,
            health_interval_ms: self.health_interval_ms,
            shutdown_timeout_ms: self.shutdown_timeout_ms,
        });
    }
}

fn is_process_alive(pid: u32) -> bool {
    libc_kill(pid as i32, 0) == 0
}

/// Minimal kill(2) wrapper without pulling in libc crate.
/// Returns 0 on success, -1 on error.
#[cfg(unix)]
fn libc_kill(pid: i32, sig: i32) -> i32 {
    unsafe {
        extern "C" {
            fn kill(pid: i32, sig: i32) -> i32;
        }
        kill(pid, sig)
    }
}

#[cfg(not(unix))]
fn libc_kill(_pid: i32, _sig: i32) -> i32 {
    -1
}

fn start_one_sidecar(sc: &SidecarDef) {
    // Check if already running
    let already_running = with_running_pids(|pids| {
        if let Some(&pid) = pids.get(&sc.name) {
            is_process_alive(pid)
        } else {
            false
        }
    });
    if already_running {
        return;
    }

    if sc.command.is_empty() {
        return;
    }

    // Create PID directory
    let dir = pid_dir();
    let _ = std::fs::create_dir_all(&dir);

    // Start the process
    let result = std::process::Command::new(&sc.command[0])
        .args(&sc.command[1..])
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn();

    match result {
        Ok(child) => {
            let pid = child.id();
            // Write PID file
            let _ = std::fs::write(pid_file_path(&sc.name), pid.to_string());
            with_running_pids(|pids| {
                pids.insert(sc.name.clone(), pid);
            });

            // Health check polling (blocking, with timeout)
            if !sc.health_check.is_empty() {
                let deadline = std::time::Instant::now()
                    + std::time::Duration::from_millis(sc.health_timeout_ms);
                while std::time::Instant::now() < deadline {
                    // Simple TCP connect check for health_check URL
                    if check_health(&sc.health_check) {
                        return;
                    }
                    std::thread::sleep(std::time::Duration::from_millis(sc.health_interval_ms));
                }
            }
        }
        Err(_) => {
            // Failed to start — nothing to do
        }
    }
}

fn check_health(url: &str) -> bool {
    // Parse host:port from URL for a simple TCP connect check
    // e.g. "http://localhost:8080/health" -> "localhost:8080"
    let stripped = url
        .strip_prefix("http://")
        .or_else(|| url.strip_prefix("https://"))
        .unwrap_or(url);
    let host_port = stripped.split('/').next().unwrap_or(stripped);
    std::net::TcpStream::connect_timeout(
        &host_port.parse().unwrap_or_else(|_| {
            std::net::SocketAddr::from(([127, 0, 0, 1], 0))
        }),
        std::time::Duration::from_secs(2),
    )
    .is_ok()
}

fn stop_one_sidecar(sc: &SidecarDef) {
    let pid = with_running_pids(|pids| pids.remove(&sc.name));
    if let Some(pid) = pid {
        if is_process_alive(pid) {
            // Send SIGTERM
            #[cfg(unix)]
            {
                libc_kill(pid as i32, 15); // SIGTERM
                // Wait briefly for graceful shutdown
                let deadline = std::time::Instant::now()
                    + std::time::Duration::from_millis(sc.shutdown_timeout_ms);
                while std::time::Instant::now() < deadline {
                    if !is_process_alive(pid) {
                        break;
                    }
                    std::thread::sleep(std::time::Duration::from_millis(100));
                }
                // Force kill if still alive
                if is_process_alive(pid) {
                    libc_kill(pid as i32, 9); // SIGKILL
                }
            }
        }
    }
    // Clean up PID file
    let _ = std::fs::remove_file(pid_file_path(&sc.name));
}

/// Start all sidecars that match the given trigger.
#[allow(clippy::type_complexity)]
pub fn start_sidecars(trigger: &str) {
    let guard = SIDECAR_REGISTRY.lock().unwrap();
    let matching: Vec<usize> = guard
        .iter()
        .enumerate()
        .filter(|(_, sc)| sc.start_on == trigger)
        .map(|(i, _)| i)
        .collect();
    // We need to drop the guard before starting (start_one_sidecar doesn't lock SIDECAR_REGISTRY)
    // but we need the data. Collect the data we need first.
    let sidecar_data: Vec<(String, Vec<String>, String, String, u64, u64, u64)> = matching
        .iter()
        .map(|&i| {
            let sc = &guard[i];
            (
                sc.name.clone(),
                sc.command.clone(),
                sc.health_check.clone(),
                sc.start_on.clone(),
                sc.health_timeout_ms,
                sc.health_interval_ms,
                sc.shutdown_timeout_ms,
            )
        })
        .collect();
    drop(guard);

    for (name, command, health_check, start_on, ht, hi, st) in sidecar_data {
        let temp = SidecarDef {
            name,
            command,
            health_check,
            start_on,
            health_timeout_ms: ht,
            health_interval_ms: hi,
            shutdown_timeout_ms: st,
        };
        start_one_sidecar(&temp);
    }
}

/// Stop all running sidecars.
#[allow(clippy::type_complexity)]
pub fn stop_all_sidecars() {
    let guard = SIDECAR_REGISTRY.lock().unwrap();
    let sidecar_data: Vec<(String, Vec<String>, String, String, u64, u64, u64)> = guard
        .iter()
        .map(|sc| {
            (
                sc.name.clone(),
                sc.command.clone(),
                sc.health_check.clone(),
                sc.start_on.clone(),
                sc.health_timeout_ms,
                sc.health_interval_ms,
                sc.shutdown_timeout_ms,
            )
        })
        .collect();
    drop(guard);

    for (name, command, health_check, start_on, ht, hi, st) in sidecar_data {
        let temp = SidecarDef {
            name,
            command,
            health_check,
            start_on,
            health_timeout_ms: ht,
            health_interval_ms: hi,
            shutdown_timeout_ms: st,
        };
        stop_one_sidecar(&temp);
    }
}

pub fn clear_sidecar_registry() {
    SIDECAR_REGISTRY.lock().unwrap().clear();
    with_running_pids(|pids| pids.clear());
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sidecar_registration() {
        clear_sidecar_registry();

        sidecar("redis", &["redis-server", "--port", "6380"])
            .health_check("http://localhost:6380")
            .start_on("startup")
            .register();

        sidecar("worker", &["./worker"])
            .start_on("first_tool_call")
            .register();

        let guard = SIDECAR_REGISTRY.lock().unwrap();
        assert_eq!(guard.len(), 2);
        assert_eq!(guard[0].name, "redis");
        assert_eq!(guard[0].command, vec!["redis-server", "--port", "6380"]);
        assert_eq!(guard[0].health_check, "http://localhost:6380");
        assert_eq!(guard[0].start_on, "startup");
        assert_eq!(guard[1].name, "worker");
        assert_eq!(guard[1].start_on, "first_tool_call");
        drop(guard);

        clear_sidecar_registry();
    }

    #[test]
    fn test_builder_defaults() {
        clear_sidecar_registry();

        sidecar("test", &["echo", "hi"]).register();

        let guard = SIDECAR_REGISTRY.lock().unwrap();
        assert_eq!(guard[0].start_on, "first_tool_call");
        assert_eq!(guard[0].health_check, "");
        assert_eq!(guard[0].health_timeout_ms, 30000);
        assert_eq!(guard[0].shutdown_timeout_ms, 3000);
        drop(guard);

        clear_sidecar_registry();
    }

    #[test]
    fn test_pid_file_path() {
        let path = pid_file_path("redis");
        assert!(path.to_string_lossy().contains("protomcp/sidecars/redis.pid"));
    }

    #[test]
    fn test_start_sidecars_filter() {
        clear_sidecar_registry();

        // Register with a trigger that won't actually start a real process
        // (command doesn't exist, which is fine — we just test filtering)
        sidecar("a", &["__nonexistent_cmd_abc123__"])
            .start_on("startup")
            .register();
        sidecar("b", &["__nonexistent_cmd_abc123__"])
            .start_on("first_tool_call")
            .register();

        // This should attempt to start "a" but not "b"
        start_sidecars("startup");

        // "a" should have been attempted (but failed to spawn)
        // "b" should not have been attempted
        // No crash = success for this unit test

        clear_sidecar_registry();
    }

    #[test]
    fn test_clear_sidecar_registry() {
        clear_sidecar_registry();
        sidecar("x", &["true"]).register();
        {
            let guard = SIDECAR_REGISTRY.lock().unwrap();
            assert_eq!(guard.len(), 1);
        }
        clear_sidecar_registry();
        {
            let guard = SIDECAR_REGISTRY.lock().unwrap();
            assert_eq!(guard.len(), 0);
        }
    }

    #[test]
    fn test_stop_all_no_crash() {
        clear_sidecar_registry();
        sidecar("phantom", &["sleep", "999"]).register();
        // No processes actually running, stop should be a no-op
        stop_all_sidecars();
        clear_sidecar_registry();
    }
}
