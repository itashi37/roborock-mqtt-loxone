export interface ConsumableStatus {
  main_brush_work_time: number;
  side_brush_work_time: number;
  filter_work_time: number;
  sensor_dirty_time: number;
  dust_collection_work_times: number;
}

export interface ConsumablePercents {
  main_brush: number;
  side_brush: number;
  filter: number;
  sensor: number;
  dust_collection: number;
}

export interface VacuumStatus {
  state: string;
  battery: number;
  fan_speed: string;
  mop_mode: string;
  water_box: string;
  clean_time: number;
  clean_area: number;
  error_code: number;
  error: string;
  dock_error_status?: number;
  in_cleaning: boolean;
  consumables: ConsumableStatus;
  consumable_percents: ConsumablePercents;
  // Completion percentage reported directly by the robot (newer models only,
  // set while cleaning). Distinct from the estimated remaining_minutes below.
  clean_percent?: number;
  // Estimated from previous runs of the same program (absent until known).
  program?: string; // scene:<id> | seg:<rooms> | full | zone | segment
  remaining_minutes?: number;
  time_completed?: string; // RFC3339 with timezone offset
  recorded_minutes?: number; // reference duration the estimate is based on
}

export interface DeviceSummary {
  slug: string;
  name: string;
  model: string;
  online: boolean;
  status: VacuumStatus | null;
}

export interface SSEEvent {
  device: string;
  state: string;
  battery: number;
  fan_speed: string;
  mop_mode: string;
  water_box: string;
  clean_time: number;
  clean_area: number;
  error_code: number;
  error: string;
  in_cleaning: boolean;
  consumables: ConsumableStatus;
}

export const fanSpeeds = ['quiet', 'balanced', 'turbo', 'max'] as const;
export type FanSpeed = typeof fanSpeeds[number];

export const mopModes = ['standard', 'deep', 'deep_plus'] as const;
export type MopMode = typeof mopModes[number];

export function formatCleanTime(seconds: number): string {
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  if (mins === 0) return `${secs}s`;
  return `${mins}m ${secs}s`;
}

export function formatCleanArea(area: number): string {
  return `${(area / 1000000).toFixed(1)} m²`;
}

export function formatDisplayName(name: string): string {
  return name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}
