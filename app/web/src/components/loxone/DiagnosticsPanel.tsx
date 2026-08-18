import { Bug, Radio } from 'lucide-react';
import { formatActivity } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneCore, LoxoneMQTTTest } from '@/types/loxone';
import { EmptyState, formatTimestamp, relativeTimestamp, Spinner, StatusBadge } from './LoxoneUI';

interface Props {
  core: LoxoneCore;
  activity?: LoxoneActivity;
  lastCommand?: LoxoneActivity;
  activities: LoxoneActivity[];
  mqttTest?: LoxoneMQTTTest;
  testing: boolean;
  onTestMQTT: () => void;
}

export function DiagnosticsPanel({ core, activity, lastCommand, activities, mqttTest, testing, onTestMQTT }: Props) {
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <DiagnosticValue label="Last command" value={lastCommand ? formatActivity(lastCommand) : 'None'} />
      <DiagnosticValue label="Last activity" value={activity ? formatActivity(activity) : 'None'} />
      <DiagnosticValue label="Current error" value={core.error_code ? `Code ${core.error_code}` : 'None'} bad={Boolean(core.error_code)} />
      <div className="rounded-lg border border-border bg-background p-3">
        <p className="text-xs uppercase tracking-wide text-muted-foreground">MQTT loopback</p>
        <div className="mt-2 flex items-center justify-between gap-2">
          {!mqttTest ? <StatusBadge>Not tested</StatusBadge> : <StatusBadge tone={mqttTest.ok ? 'green' : 'red'}>{mqttTest.ok ? 'Passed' : 'Failed'}</StatusBadge>}
          <button onClick={onTestMQTT} disabled={testing} className="touch-target inline-flex items-center gap-2 rounded-lg border border-border px-2 text-xs hover:bg-accent disabled:opacity-50">{testing ? <Spinner label="Testing" /> : <><Radio className="h-4 w-4" /> Test</>}</button>
        </div>
        {mqttTest && <p className="mt-2 text-xs text-muted-foreground" title={formatTimestamp(mqttTest.tested_at)}>{mqttTest.message} · {relativeTimestamp(mqttTest.tested_at)}</p>}
      </div>
    </div>

    <div>
      <h4 className="mb-2 text-sm font-medium">Recent history</h4>
      {activities.length === 0 ? <EmptyState>No command or event has been recorded yet.</EmptyState> : <ol className="max-h-72 divide-y divide-border overflow-y-auto rounded-xl border border-border">
        {activities.map((item, index) => <li key={`${item.type}-${item.ts}-${index}`} className="grid gap-1 p-3 text-sm sm:grid-cols-[160px_110px_1fr]">
          <time className="text-xs text-muted-foreground" dateTime={new Date(item.ts * 1000).toISOString()}>{formatTimestamp(item.ts)}</time>
          <StatusBadge tone={item.type === 'command' ? 'blue' : item.event === 'error' || item.event === 'stuck' ? 'red' : 'neutral'}>{item.type}</StatusBadge>
          <span className="break-words">{formatActivity(item)}</span>
        </li>)}
      </ol>}
    </div>

    <details className="rounded-xl border border-border bg-background">
      <summary className="flex cursor-pointer items-center gap-2 p-3 text-sm font-medium"><Bug className="h-4 w-4" /> Advanced debug payloads</summary>
      <div className="grid gap-3 border-t border-border p-3 lg:grid-cols-2">
        <RawPayload label="Last /core known by the UI" value={core} />
        <RawPayload label="Last /activity received" value={activity ?? null} />
      </div>
    </details>
  </div>;
}

function DiagnosticValue({ label, value, bad = false }: { label: string; value: string; bad?: boolean }) {
  return <div className="rounded-lg border border-border bg-background p-3"><p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p><p className={`mt-2 break-words text-sm font-medium ${bad ? 'text-red-500' : ''}`}>{value}</p></div>;
}

function RawPayload({ label, value }: { label: string; value: unknown }) {
  return <div className="min-w-0"><p className="mb-2 text-xs font-medium text-muted-foreground">{label}</p><pre className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">{JSON.stringify(value, null, 2)}</pre></div>;
}
