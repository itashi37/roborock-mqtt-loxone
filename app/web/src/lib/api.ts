import type { DeviceSummary } from '@/types/status';
import type { AdvancedDiagnosticsResponse, LoxoneExportSelection, LoxoneIntegration, LoxoneMQTTTest, LoxoneRoom } from '@/types/loxone';
import type { SystemStatus, UpdateInfo, UpdateOperation } from '@/types/system';

export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

export async function fetchSystemStatus(): Promise<SystemStatus> {
  const response = await fetch(`${API_BASE}/system/status`, { cache: 'no-store' });
  if (!response.ok) throw new Error('Failed to load system status');
  return response.json();
}

export async function checkForUpdates(channel: 'stable' | 'edge'): Promise<UpdateInfo> {
  const response = await fetch(`${API_BASE}/system/updates/check`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ channel }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Update check failed');
  return data;
}

export async function fetchUpdateOperation(): Promise<UpdateOperation> {
  const response = await fetch(`${API_BASE}/system/updates/operation`, { cache: 'no-store' });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Isolated updater is unavailable');
  return data;
}

export async function installUpdate(channel: 'stable' | 'edge'): Promise<UpdateOperation> {
  const response = await fetch(`${API_BASE}/system/updates/install`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Roborock-Intent': 'install-update' },
    body: JSON.stringify({ channel }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Update installation failed to start');
  return data;
}

export interface AuthStatus {
  authenticated: boolean;
  user?: string;
  devices?: number;
}

export interface SetupStatus {
  setup_complete: boolean;
  authenticated: boolean;
  roborock_username: string;
  mqtt: {
    enabled: boolean; url: string; retain: boolean; topic: string; qos: number;
    username: string; tls: boolean; password_configured: boolean;
  };
  loxone: {
    enabled: boolean; topic: string;
    devices?: Record<string, { mqtt?: boolean; direct?: boolean }>;
    direct: {
      enabled: boolean; scheme: string; host: string; port: number; username: string;
      timeout_seconds: number; max_retries: number; retry_delay_ms: number;
      input_prefix: string; api_username: string; allowed_cidrs?: string[];
      allow_get_commands: boolean; rate_limit_per_minute: number;
      password_configured: boolean; api_token_configured: boolean;
    };
  };
  mqtt_diagnostics: { enabled: boolean; connected: boolean; last_error?: string; subscriptions: number };
  direct_diagnostics?: { queued: number; last_error?: string; last_success_at?: string };
}

export async function fetchSetupStatus(): Promise<SetupStatus> {
  const response = await fetch(`${API_BASE}/setup/status`, { cache: 'no-store' });
  if (!response.ok) throw new Error('Failed to load integration settings');
  return response.json();
}

export async function saveSetupSettings(payload: unknown): Promise<SetupStatus> {
  const response = await fetch(`${API_BASE}/setup/settings`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to save settings');
  return data;
}

export async function testMQTTSettings(payload: unknown): Promise<void> {
  const response = await fetch(`${API_BASE}/setup/mqtt/test`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'MQTT connection failed');
}

export async function testDirectSettings(payload: unknown): Promise<void> {
  const response = await fetch(`${API_BASE}/setup/direct/test`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Miniserver connection failed');
}

export async function rotateDirectToken(): Promise<string> {
  const response = await fetch(`${API_BASE}/setup/direct/token/rotate`, { method: 'POST' });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to rotate token');
  return data.token;
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const response = await fetch(`${API_BASE}/auth/status`);
  if (!response.ok) throw new Error('Failed to get auth status');
  return response.json();
}

export async function requestCode(username?: string): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/request-code`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username }),
  });
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error || 'Failed to request code');
  }
}

export async function loginWithCode(code: string): Promise<AuthStatus> {
  const response = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Login failed');
  return data;
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/auth/logout`, { method: 'POST' });
}

export async function fetchDevices(): Promise<DeviceSummary[]> {
  const response = await fetch(`${API_BASE}/devices`);
  if (!response.ok) throw new Error('Failed to fetch devices');
  return response.json();
}

export async function startCleaning(slug: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/start`, { method: 'POST' });
  if (!response.ok) throw new Error('Failed to start');
}

export async function pauseCleaning(slug: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/pause`, { method: 'POST' });
  if (!response.ok) throw new Error('Failed to pause');
}

export async function dockVacuum(slug: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/dock`, { method: 'POST' });
  if (!response.ok) throw new Error('Failed to dock');
}

export async function setFanSpeed(slug: string, speed: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/fan-speed`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ speed }),
  });
  if (!response.ok) throw new Error('Failed to set fan speed');
}

export async function setMopMode(slug: string, mode: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/mop-mode`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) throw new Error('Failed to set mop mode');
}

export interface SceneInfo {
  id: number;
  name: string;
  recorded_minutes?: number; // typical run duration from previous runs (absent until known)
}

export async function fetchScenes(slug: string): Promise<SceneInfo[]> {
  const response = await fetch(`${API_BASE}/devices/${slug}/scenes`);
  if (!response.ok) throw new Error('Failed to fetch scenes');
  return response.json();
}

export async function executeScene(slug: string, sceneId: number): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/scenes/${sceneId}/execute`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to execute scene');
}

// --- Schedule ---

import type { ScheduleResponse, DeviceSchedule } from '@/types/schedule';

export async function fetchSchedule(slug: string): Promise<ScheduleResponse> {
  const response = await fetch(`${API_BASE}/devices/${slug}/schedule`);
  if (!response.ok) throw new Error('Failed to fetch schedule');
  return response.json();
}

export async function fetchScheduleStatus(): Promise<any> {
  const response = await fetch(`${API_BASE}/schedule/status`);
  if (!response.ok) throw new Error('Failed to fetch schedule status');
  return response.json();
}

export async function saveSchedule(slug: string, schedule: DeviceSchedule): Promise<any> {
  const response = await fetch(`${API_BASE}/devices/${slug}/schedule`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(schedule),
  });
  if (!response.ok) throw new Error('Failed to save schedule');
  return response.json();
}

export async function deleteSchedule(slug: string): Promise<any> {
  const response = await fetch(`${API_BASE}/devices/${slug}/schedule`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to delete schedule');
  return response.json();
}

export async function resetConsumable(slug: string, name: string): Promise<any> {
  const response = await fetch(`${API_BASE}/devices/${slug}/consumables/${name}/reset`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to reset consumable');
  return response.json();
}

export async function setNotAtHome(enabled: boolean): Promise<any> {
  const response = await fetch(`${API_BASE}/not-at-home`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  });
  if (!response.ok) throw new Error('Failed to update not-at-home');
  return response.json();
}

// --- Loxone integration ---

export async function fetchLoxoneIntegration(): Promise<LoxoneIntegration> {
  const response = await fetch(`${API_BASE}/loxone/integration`);
  if (!response.ok) throw new Error('Failed to load Loxone integration');
  return response.json();
}

export async function fetchAdvancedDiagnostics(slug: string): Promise<AdvancedDiagnosticsResponse> {
  const response = await fetch(`${API_BASE}/devices/${slug}/advanced-diagnostics`, { cache: 'no-store' });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to query app_get_init_status');
  return data;
}

export async function saveLoxoneRoomOverride(slug: string, roomId: number, name: string): Promise<LoxoneRoom[]> {
  const response = await fetch(`${API_BASE}/loxone/devices/${slug}/rooms/${roomId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to save room name');
  return data.rooms;
}

export async function deleteLoxoneRoomOverride(slug: string, roomId: number): Promise<LoxoneRoom[]> {
  const response = await fetch(`${API_BASE}/loxone/devices/${slug}/rooms/${roomId}`, { method: 'DELETE' });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to delete room override');
  return data.rooms;
}

export async function testLoxoneCommand(slug: string, command: string): Promise<void> {
  const response = await fetch(`${API_BASE}/loxone/devices/${slug}/command`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Failed to publish command');
}

export async function testLoxoneMQTT(): Promise<LoxoneMQTTTest> {
  const response = await fetch(`${API_BASE}/loxone/mqtt-test`, { method: 'POST' });
  const data = await response.json();
  if (!response.ok && typeof data?.ok !== 'boolean') throw new Error(data.message || data.error || 'MQTT test failed');
  return data;
}

export async function downloadLoxoneIntegration(robots: LoxoneExportSelection[]): Promise<void> {
  const response = await fetch(`${API_BASE}/loxone/export`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ robots }),
  });
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error || 'Failed to generate integration pack');
  }
  const blob = await response.blob();
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? 'roborock-mqtt-loxone-integration.zip';
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
