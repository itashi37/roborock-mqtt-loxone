import { Play, Shapes } from 'lucide-react';
import { latestSceneActivity } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneExportSelection, LoxoneScene } from '@/types/loxone';
import { displayActivity, displayRobotState, EmptyState, relativeTimestamp, Spinner, StatusBadge } from './LoxoneUI';

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
  if (scenes.length === 0) return <EmptyState>Créez vos programmes dans l’application Roborock. Ils apparaîtront ensuite ici pour pouvoir être lancés depuis Loxone.</EmptyState>;
  return <div className="divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200">
    {scenes.map(scene => {
      const activity = latestSceneActivity(scene, activities);
      const key = `scene:${slug}:${scene.id}`;
      return <div key={scene.id} className="flex flex-col gap-3 bg-white p-4 sm:flex-row sm:items-center">
        <input type="checkbox" checked={Boolean(selected?.scene_ids.includes(scene.id))} disabled={!selected} onChange={event => onSelect(scene.id, event.target.checked)} className="h-4 w-4 accent-emerald-600" aria-label={`Inclure ${scene.name} dans le dossier`} />
        <span className="hidden h-9 w-9 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 sm:flex"><Shapes className="h-4 w-4" /></span>
        <div className="min-w-0 flex-1"><strong className="block truncate text-sm font-medium text-slate-700">{scene.name}</strong><span className="mt-1 block text-[10px] text-slate-400">Programme n° {scene.id}</span></div>
        <div className="sm:mr-2 sm:text-right">{activity ? <><StatusBadge tone={activity.state === 'failed' ? 'red' : activity.state === 'completed' ? 'green' : 'blue'}>{displayRobotState(activity.state ?? 'unknown')}</StatusBadge><p className="mt-1 text-[10px] text-slate-400" title={displayActivity(activity)}>{relativeTimestamp(activity.ts)}</p></> : <span className="text-[10px] text-slate-400">Aucun lancement récent</span>}</div>
        <button onClick={() => onTest(scene)} disabled={busy.has(key)} className="inline-flex min-h-9 items-center justify-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50/50 px-3 text-xs font-medium text-emerald-800 hover:bg-emerald-100 disabled:opacity-50">
          {busy.has(key) ? <Spinner label="Envoi…" /> : <><Play className="h-3.5 w-3.5" />Lancer</>}
        </button>
      </div>;
    })}
  </div>;
}
