import { Activity, AlertTriangle, Bot, CheckCircle2, Radio, Server } from 'lucide-react';
import type { FleetHealth, LoxoneActivity, LoxoneMQTTTest } from '@/types/loxone';
import { formatActivity } from '@/lib/loxone';
import { relativeTimestamp, StatusBadge } from './LoxoneUI';

interface DashboardProps {
  bridgeStarted: boolean;
  streamConnected: boolean;
  mqttTest?: LoxoneMQTTTest;
  onlineRobots: number;
  robotCount: number;
  subscriptions: number;
  limit: number;
  latest?: { name: string; activity: LoxoneActivity };
  fleet?: FleetHealth;
}

export function LoxoneDashboard(props: DashboardProps) {
  return <section className="space-y-4" aria-label="Integration summary">
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <SummaryCard icon={<Server className="h-5 w-5" />} label="Bridge">
        <StatusBadge tone={props.bridgeStarted ? 'green' : 'red'}>
          {props.bridgeStarted ? 'Online' : 'Offline'}
        </StatusBadge>
        <span className="text-xs text-muted-foreground">{props.streamConnected ? 'Live UI connected' : 'Live UI disconnected'}</span>
      </SummaryCard>
      <SummaryCard icon={<Radio className="h-5 w-5" />} label="MQTT">
        {!props.mqttTest ? <StatusBadge tone="neutral">Not tested</StatusBadge> : props.mqttTest.ok
          ? <StatusBadge tone="green"><CheckCircle2 className="h-3 w-3" /> Test passed</StatusBadge>
          : <StatusBadge tone="red"><AlertTriangle className="h-3 w-3" /> Test failed</StatusBadge>}
        {props.mqttTest && <span className="text-xs text-muted-foreground">{relativeTimestamp(props.mqttTest.tested_at)}</span>}
      </SummaryCard>
      <SummaryCard icon={<Bot className="h-5 w-5" />} label="Robots">
        <strong className="text-lg">{props.robotCount}</strong>
        <span className="text-xs text-muted-foreground">{props.onlineRobots} online · {props.fleet?.in_error ?? 0} errors</span>
        {props.fleet && <StatusBadge tone={props.fleet.health === 'healthy' ? 'green' : props.fleet.health === 'degraded' ? 'amber' : 'red'}>{props.fleet.health}</StatusBadge>}
      </SummaryCard>
      <SummaryCard icon={<Activity className="h-5 w-5" />} label="Subscriptions">
        <strong className={props.subscriptions > props.limit ? 'text-amber-500' : ''}>{props.subscriptions} / {props.limit}</strong>
        <div className="h-2 w-full overflow-hidden rounded-full bg-muted" role="progressbar" aria-label="Subscription budget" aria-valuemin={0} aria-valuemax={props.limit} aria-valuenow={props.subscriptions}>
          <div className={`h-full rounded-full ${props.subscriptions > props.limit ? 'bg-amber-500' : 'bg-primary'}`} style={{ width: `${Math.min(100, props.subscriptions / props.limit * 100)}%` }} />
        </div>
      </SummaryCard>
    </div>
    <div className="rounded-xl border border-border bg-card p-4">
      <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Latest global activity</p>
      {props.latest ? <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
        <strong>{props.latest.name}</strong><span className="text-muted-foreground">·</span><span>{formatActivity(props.latest.activity)}</span><span className="ml-auto text-xs text-muted-foreground">{relativeTimestamp(props.latest.activity.ts)}</span>
      </div> : <p className="mt-2 text-sm text-muted-foreground">No activity recorded in this session.</p>}
    </div>
  </section>;
}

function SummaryCard({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return <div className="rounded-xl border border-border bg-card p-4">
    <div className="mb-3 flex items-center gap-2 text-muted-foreground">{icon}<span className="text-xs font-medium uppercase tracking-wide">{label}</span></div>
    <div className="flex min-h-10 flex-col justify-center gap-1">{children}</div>
  </div>;
}
