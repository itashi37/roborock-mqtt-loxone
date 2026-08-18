import { AlertTriangle, Download } from 'lucide-react';
import type { LoxoneExportSelection, LoxoneRobot } from '@/types/loxone';
import { Spinner, StatusBadge } from './LoxoneUI';

interface Props {
  robots: LoxoneRobot[];
  selection: LoxoneExportSelection[];
  perRobot: number;
  limit: number;
  required: number;
  exceeds: boolean;
  exporting: boolean;
  onRobotSelected: (slug: string, selected: boolean) => void;
  onDownload: () => void;
}

export function LoxoneExportPanel({ robots, selection, perRobot, limit, required, exceeds, exporting, onRobotSelected, onDownload }: Props) {
  return <section className="rounded-2xl border border-border bg-card p-5 shadow-sm" aria-labelledby="loxone-export-heading">
    <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2"><h2 id="loxone-export-heading" className="text-lg font-semibold">Download Loxone Integration</h2><StatusBadge tone="blue">ZIP</StatusBadge></div>
        <p className="mt-1 text-sm text-muted-foreground">Select the robots, commandable rooms and scenes to include. No MQTT or Roborock credentials are exported.</p>
        <div className="mt-4 grid gap-2 sm:grid-cols-2">
          {robots.map(robot => {
            const item = selection.find(selected => selected.slug === robot.slug);
            return <label key={robot.slug} className={`flex cursor-pointer items-center gap-3 rounded-xl border p-3 ${item ? 'border-primary/40 bg-primary/5' : 'border-border'}`}>
              <input type="checkbox" checked={Boolean(item)} onChange={event => onRobotSelected(robot.slug, event.target.checked)} className="h-4 w-4" />
              <span className="min-w-0 flex-1"><strong className="block truncate text-sm">{robot.name}</strong><span className="text-xs text-muted-foreground">{item?.room_ids.length ?? 0} rooms · {item?.scene_ids.length ?? 0} scenes</span></span>
            </label>;
          })}
        </div>
        <div className={`mt-4 rounded-xl border p-4 ${exceeds ? 'border-amber-500/40 bg-amber-500/10' : 'border-border bg-background'}`}>
          <div className="flex flex-wrap items-center justify-between gap-2"><strong>Subscription budget: {required} / {limit}</strong>{exceeds ? <StatusBadge tone="amber"><AlertTriangle className="h-3 w-3" /> Limit exceeded</StatusBadge> : <StatusBadge tone="green">Within limit</StatusBadge>}</div>
          <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted"><div className={`h-full rounded-full ${exceeds ? 'bg-amber-500' : 'bg-primary'}`} style={{ width: `${Math.min(100, required / limit * 100)}%` }} /></div>
          <p className="mt-3 text-sm"><strong>Calculation:</strong> {selection.length} selected robot{selection.length === 1 ? '' : 's'} × {perRobot} subscriptions each (<code>/core</code> + <code>/activity</code>) = <strong>{required} subscriptions</strong>.</p>
          <p className="mt-1 text-xs text-muted-foreground">The <code>/command</code> topic is a publication and does not consume a Loxone MQTT subscription.</p>
          {exceeds && <p className="mt-2 text-sm text-amber-700 dark:text-amber-300">The standard configuration exceeds Loxone’s limit of {limit}. The pack can still be generated.</p>}
        </div>
      </div>
      <button onClick={onDownload} disabled={exporting || selection.length === 0} className="touch-target inline-flex shrink-0 items-center justify-center gap-2 rounded-xl bg-primary px-5 py-3 font-medium text-primary-foreground shadow-sm hover:opacity-90 disabled:opacity-50">
        {exporting ? <Spinner label="Generating" /> : <><Download className="h-5 w-5" /> Download Loxone Integration</>}
      </button>
    </div>
  </section>;
}
