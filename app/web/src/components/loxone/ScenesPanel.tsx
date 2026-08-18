import { Play } from 'lucide-react';
import { latestSceneActivity, formatActivity } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneExportSelection, LoxoneScene } from '@/types/loxone';
import { EmptyState, relativeTimestamp, Spinner, StatusBadge } from './LoxoneUI';

interface Props {
  slug: string;
  scenes: LoxoneScene[];
  activities: LoxoneActivity[];
  selected?: LoxoneExportSelection;
  busy: Set<string>;
  onSelect: (id: number, selected: boolean) => void;
  onTest: (scene: LoxoneScene) => void;
}

export function ScenesPanel({ slug, scenes, activities, selected, busy, onSelect, onTest }: Props) {
  if (scenes.length === 0) return <EmptyState>No Roborock scene is available for this robot.</EmptyState>;
  return <div className="divide-y divide-border overflow-hidden rounded-xl border border-border">
    {scenes.map(scene => {
      const activity = latestSceneActivity(scene, activities);
      const key = `scene:${slug}:${scene.id}`;
      return <div key={scene.id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center">
        <input type="checkbox" checked={Boolean(selected?.scene_ids.includes(scene.id))} disabled={!selected} onChange={event => onSelect(scene.id, event.target.checked)} className="h-4 w-4" aria-label={`Include ${scene.name} in export`} />
        <div className="min-w-0 flex-1"><strong className="block truncate">{scene.name}</strong><span className="text-xs text-muted-foreground">Roborock ID {scene.id}</span></div>
        <div className="sm:text-right">{activity ? <><StatusBadge tone={activity.state === 'failed' ? 'red' : activity.state === 'completed' ? 'green' : 'blue'}>{activity.state ?? 'unknown'}</StatusBadge><p className="mt-1 text-xs text-muted-foreground" title={formatActivity(activity)}>{relativeTimestamp(activity.ts)}</p></> : <span className="text-xs text-muted-foreground">Never run in available history</span>}</div>
        <button onClick={() => onTest(scene)} disabled={busy.has(key)} className="touch-target inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 text-sm hover:bg-accent disabled:opacity-50">
          {busy.has(key) ? <Spinner label="Testing" /> : <><Play className="h-4 w-4" /> Test</>}
        </button>
      </div>;
    })}
  </div>;
}
