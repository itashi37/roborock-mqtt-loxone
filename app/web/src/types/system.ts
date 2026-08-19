export interface HealthComponent {
  healthy: boolean;
  required: boolean;
  detail?: string;
  last_activity?: string;
}

export interface HealthReport {
  status: 'healthy' | 'degraded' | 'unhealthy' | string;
  live: boolean;
  ready: boolean;
  observed_at?: string;
  uptime_seconds: number;
  reasons?: string[];
  components?: Record<string, HealthComponent>;
  last_action?: string;
  last_action_at?: string;
  last_watchdog_reason?: string;
  last_watchdog_restart?: string;
  restart_suppressed?: boolean;
}

export interface UpdateInfo {
  channel: 'stable' | 'edge' | string;
  current_version: string;
  latest_version?: string;
  published_at?: string;
  release_notes?: string;
  release_url?: string;
  available: boolean;
  checked_at?: string;
  error?: string;
}

export interface SystemStatus {
  product: string;
  version: string;
  git_commit: string;
  build_time: string;
  go_version: string;
  architecture: string;
  channel: 'stable' | 'edge' | string;
  uptime_seconds: number;
  started_at: string;
  last_restart: string;
  last_watchdog_reason?: string;
  health: HealthReport;
  data_volume: { path: string; writable: boolean; free_bytes: number; error?: string };
  transports: Record<string, { enabled: boolean; connected: boolean; last_success?: string; last_error?: string }>;
  update: UpdateInfo;
}
