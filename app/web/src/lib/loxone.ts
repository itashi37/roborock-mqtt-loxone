import type { LoxoneActivity, LoxoneExportSelection, LoxoneIntegration, LoxoneRoom, LoxoneScene } from '@/types/loxone';

export function subscriptionBudget(selectedRobots: number, perRobot = 2, limit = 16) {
  const required = selectedRobots * perRobot;
  return { required, limit, exceeds: required > limit };
}

export function defaultLoxoneSelection(integration: LoxoneIntegration): LoxoneExportSelection[] {
  return integration.robots.map(robot => ({
    slug: robot.slug,
    room_ids: robot.rooms.map(room => room.id),
    scene_ids: robot.scenes.map(scene => scene.id),
  }));
}

export function updateSelectedID(values: number[], id: number, selected: boolean): number[] {
  if (selected) return values.includes(id) ? values : [...values, id];
  return values.filter(value => value !== id);
}

export interface RoomDraftValidation {
  value: string;
  error?: string;
  conflict: boolean;
}

export function validateRoomDrafts(rooms: LoxoneRoom[], drafts: Record<string, string>): Record<number, RoomDraftValidation> {
  const result: Record<number, RoomDraftValidation> = {};
  const counts = new Map<string, number>();

  for (const room of rooms) {
    const value = (drafts[String(room.id)] ?? room.override_name ?? room.effective_name).trim();
    if (value) counts.set(value.toLocaleLowerCase(), (counts.get(value.toLocaleLowerCase()) ?? 0) + 1);
    result[room.id] = {
      value,
      error: value.length === 0 ? 'A name is required.' : [...value].length > 80 ? 'The name must not exceed 80 characters.' : undefined,
      conflict: false,
    };
  }

  for (const room of rooms) {
    const current = result[room.id];
    current.conflict = Boolean(current.value) && (counts.get(current.value.toLocaleLowerCase()) ?? 0) > 1;
    if (current.conflict) current.error = `“${current.value}” is already used by another commandable segment.`;
  }
  return result;
}

export function activityKey(activity: LoxoneActivity): string {
  return [activity.type, activity.ts, activity.id, activity.command, activity.state, activity.event, activity.room_id].join(':');
}

export function mergeActivities(current: LoxoneActivity[], incoming: LoxoneActivity[], limit = 50): LoxoneActivity[] {
  const byKey = new Map<string, LoxoneActivity>();
  for (const activity of [...current, ...incoming]) byKey.set(activityKey(activity), activity);
  return [...byKey.values()].sort((a, b) => b.ts - a.ts).slice(0, limit);
}

export function latestActivity(integration: LoxoneIntegration): { slug: string; name: string; activity: LoxoneActivity } | undefined {
  const candidates = integration.robots.flatMap(robot => {
    const activity = robot.diagnostics.last_activity ?? [...(robot.diagnostics.recent ?? [])].sort((a, b) => b.ts - a.ts)[0];
    return activity ? [{ slug: robot.slug, name: robot.name, activity }] : [];
  });
  return candidates.sort((a, b) => b.activity.ts - a.activity.ts)[0];
}

export function latestSceneActivity(scene: LoxoneScene, activities: LoxoneActivity[]): LoxoneActivity | undefined {
  const byID = scene.command.toLocaleLowerCase();
  const byName = `scene:${scene.name}`.toLocaleLowerCase();
  return activities.find(activity => activity.type === 'command' && [byID, byName].includes((activity.command ?? '').trim().toLocaleLowerCase()));
}

export function formatActivity(activity: LoxoneActivity): string {
  if (activity.type === 'command') return `${activity.command ?? 'command'} · ${activity.state ?? 'unknown'}${activity.error ? ` · ${activity.error}` : ''}`;
  return `${activity.event ?? 'event'}${activity.room_name ? ` · ${activity.room_name}` : ''}${activity.error_text ? ` · ${activity.error_text}` : ''}`;
}
