export interface LoxoneCore {
  online: number;
  state: string;
  battery: number;
  current_room_id: number;
  current_room_name: string;
  error_code: number;
  last_seen: number;
}

export interface LoxoneActivity {
  type: 'command' | 'event';
  ts: number;
  id?: string;
  command?: string;
  state?: string;
  error?: string;
  event?: string;
  room_id?: number;
  room_name?: string;
  error_code?: number;
  error_text?: string;
}

export interface LoxoneDiagnostics {
  last_activity?: LoxoneActivity;
  last_command?: LoxoneActivity;
  recent?: LoxoneActivity[];
}

export interface LoxoneRoom {
  id: number;
  roborock_name: string;
  config_name?: string;
  override_name?: string;
  effective_name: string;
  conflict: boolean;
  command: string;
}

export interface LoxoneScene {
  id: number;
  name: string;
  command: string;
}

export interface RobotCapability {
  supported: boolean | null;
  source: string;
  confidence: 'confirmed' | 'reported' | 'observed' | 'unknown';
  values?: string[];
  last_checked: string;
  reason?: string;
}

export interface RobotCapabilities {
  rooms: RobotCapability;
  scenes: RobotCapability;
  fan: RobotCapability;
  mop: RobotCapability;
  water: RobotCapability;
  locate: RobotCapability;
  dock: RobotCapability;
  dock_empty: RobotCapability;
  mop_wash: RobotCapability;
  mop_dry: RobotCapability;
  stop: RobotCapability;
}

export interface AdvancedDiagnosticsResponse {
  diagnostics: { method: string; collected_at: string; fields: Record<string, unknown> };
  capabilities: RobotCapabilities;
}

export interface LoxoneRobot {
  slug: string;
  name: string;
  model: string;
  online: boolean;
  mqtt_enabled: boolean;
  direct_enabled: boolean;
  core: LoxoneCore;
  topics: {
    core: string;
    activity: string;
    command: string;
    last_command: string;
  };
  rooms: LoxoneRoom[];
  scenes: LoxoneScene[];
  diagnostics: LoxoneDiagnostics;
  capabilities: RobotCapabilities;
  health: DeviceHealth;
  active_program?: string;
  active_scene_id?: number;
  active_scene_name?: string;
  direct_inputs?: LoxoneDirectInput[];
  direct_outputs?: LoxoneDirectOutput[];
}

export interface DeviceHealth {
  slug: string;
  online: boolean;
  in_error: boolean;
  error_code: number;
  dock_state: string;
  last_poll_attempt?: string;
  last_poll_success?: string;
  last_communication?: string;
  status_latency_ms: number;
  consecutive_failures: number;
  backoff_seconds: number;
  next_poll_at?: string;
  last_error?: string;
}

export interface FleetHealth {
  health: 'healthy' | 'degraded' | 'offline';
  updated_at: string;
  robots: number;
  online: number;
  in_error: number;
  poll_failures: number;
  devices: DeviceHealth[];
}

export interface LoxoneMQTTTest {
  ok: boolean;
  message: string;
  tested_at: number;
}

export interface DirectInputDiagnostic {
  robot: string;
  field: string;
  input: string;
  kind: 'digital' | 'analog' | 'text';
  last_value?: string;
  last_attempt?: string;
  last_success?: string;
  last_error?: string;
  consecutive_retries: number;
}

export interface DirectDiagnostics {
  last_transmission?: string;
  last_error?: string;
  inputs: DirectInputDiagnostic[];
}

export interface LoxoneDirectInput {
  name: string;
  field: string;
  kind: 'digital' | 'analog' | 'text';
}

export interface LoxoneDirectOutput {
  name: string;
  method: 'POST';
  path: string;
  command: string;
  level: 'safe' | 'advanced';
}

export interface LoxoneIntegration {
  project: string;
  upstream: string;
  enabled: boolean;
  bridge_started: boolean;
  topic: string;
  subscription_limit: number;
  subscriptions_per_robot: number;
  subscriptions_required: number;
  exceeds_limit: boolean;
  warning?: string;
  mqtt_test?: LoxoneMQTTTest;
  direct_enabled: boolean;
  direct_api_username?: string;
  direct_token_configured: boolean;
  direct_diagnostics?: DirectDiagnostics;
  health_inputs?: LoxoneDirectInput[];
  robots: LoxoneRobot[];
  fleet?: FleetHealth;
  template_status: {
    native_generation: boolean;
    format_verified: boolean;
    reason: string;
    required_samples: string[];
  };
}

export interface LoxoneExportSelection {
  slug: string;
  room_ids: number[];
  scene_ids: number[];
}
