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
  transports: Record<string, { enabled: boolean; connected: boolean; last_success?: string; last_error?: string; failed_inputs?: Array<{ name: string; field: string; kind: 'digital' | 'analog' | 'text' | string; error: string }> }>;
  update: UpdateInfo;
  update_settings: UpdateSettings;
  auto_update: AutoUpdateDiagnostics;
  supervisor: { kind: string; data_dir: string; log_mode: string; restart_supported: boolean };
}

export interface UpdateSettings {
  version: number;
  mode: 'off' | 'notify' | 'automatic';
  channel: 'stable' | 'edge';
  window_start: string;
  window_end: string;
  delay_hours: number;
  allowed_days: number[];
  prevent_robot_active: boolean;
  prevent_cleaning: boolean;
  prevent_command_in_progress: boolean;
  allow_edge_automatic: boolean;
}

export interface AutoUpdateDiagnostics {
  last_check?: string;
  last_decision?: string;
  last_version?: string;
  last_attempt?: string;
  last_operation?: string;
  last_error?: string;
  next_window?: string;
  guard?: { robot_active: boolean; cleaning: boolean; commands_in_flight: number };
}

export type UpdateStage = 'idle' | 'preparing' | 'pulling' | 'backing_up' | 'restarting' | 'validating' | 'success' | 'rollback' | 'failed';

export interface UpdateOperation {
  id?: string;
  stage: UpdateStage;
  tag?: string;
  expected_version?: string;
  previous_image?: string;
  target_image?: string;
  backup_path?: string;
  started_at?: string;
  updated_at?: string;
  completed_at?: string;
  error?: string;
  rollback_error?: string;
}
