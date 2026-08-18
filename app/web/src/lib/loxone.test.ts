import { describe, expect, it } from 'vitest';
import { defaultLoxoneSelection, latestActivity, latestSceneActivity, mergeActivities, subscriptionBudget, updateSelectedID, validateRoomDrafts } from './loxone';
import type { LoxoneIntegration, LoxoneRoom } from '@/types/loxone';

describe('Loxone integration helpers', () => {
  it('warns without blocking when subscriptions exceed 16', () => {
    expect(subscriptionBudget(8)).toEqual({ required: 16, limit: 16, exceeds: false });
    expect(subscriptionBudget(9)).toEqual({ required: 18, limit: 16, exceeds: true });
  });

  it('selects detected rooms and scenes by default', () => {
    const integration = {
      robots: [{
        slug: 'vacuum',
        rooms: [{ id: 23 }, { id: 24 }],
        scenes: [{ id: 101 }],
      }],
    } as unknown as LoxoneIntegration;
    expect(defaultLoxoneSelection(integration)).toEqual([{ slug: 'vacuum', room_ids: [23, 24], scene_ids: [101] }]);
  });

  it('updates selected IDs without duplicates', () => {
    expect(updateSelectedID([23], 23, true)).toEqual([23]);
    expect(updateSelectedID([23], 24, true)).toEqual([23, 24]);
    expect(updateSelectedID([23, 24], 23, false)).toEqual([24]);
  });

  it('validates draft names only between commandable rooms', () => {
    const rooms = [
      { id: 7, effective_name: 'Cuisine', roborock_name: 'Cuisine' },
      { id: 23, effective_name: 'Salon', roborock_name: 'Salon' },
    ] as LoxoneRoom[];
    const result = validateRoomDrafts(rooms, { '7': ' Salon ', '23': 'salon' });
    expect(result[7].conflict).toBe(true);
    expect(result[23].conflict).toBe(true);
    expect(validateRoomDrafts(rooms, { '7': '', '23': 'Salon' })[7].error).toBe('A name is required.');
    expect(validateRoomDrafts(rooms, { '7': 'Cuisine', '23': 'Salon' })[7].conflict).toBe(false);
  });

  it('deduplicates and sorts recent activity', () => {
    const old = { type: 'event', event: 'paused', ts: 10 } as const;
    const recent = { type: 'command', command: 'pause', state: 'completed', ts: 20 } as const;
    expect(mergeActivities([old], [old, recent])).toEqual([recent, old]);
  });

  it('finds the latest global and scene activities', () => {
    const sceneActivity = { type: 'command', command: 'scene_id:101', state: 'completed', ts: 30 } as const;
    const integration = {
      robots: [
        { slug: 'lower', name: 'Lower', diagnostics: { recent: [{ type: 'event', event: 'paused', ts: 20 }, { type: 'event', event: 'resumed', ts: 25 }] } },
        { slug: 'upper', name: 'Upper', diagnostics: { recent: [sceneActivity] } },
      ],
    } as unknown as LoxoneIntegration;
    expect(latestActivity(integration)?.slug).toBe('upper');
    expect(latestSceneActivity({ id: 101, name: 'Dinner', command: 'scene_id:101' }, [sceneActivity])).toEqual(sceneActivity);
  });
});
