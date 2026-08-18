import type { LoxoneExportSelection, LoxoneIntegration } from '@/types/loxone';

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
