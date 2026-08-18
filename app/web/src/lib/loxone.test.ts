import { describe, expect, it } from 'vitest';
import { defaultLoxoneSelection, subscriptionBudget, updateSelectedID } from './loxone';
import type { LoxoneIntegration } from '@/types/loxone';

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
});
