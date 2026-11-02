import type { LoxoneRobot } from '@/types/loxone';
import { CopyButton, StatusBadge } from './LoxoneUI';

export function MqttTopicsPanel({ topics }: { topics: LoxoneRobot['topics'] }) {
  return <div className="overflow-hidden rounded-xl border border-border">
    <TopicRow label="État du robot" mode="Abonnement" retained value={topics.core} />
    <TopicRow label="Activité" mode="Abonnement" value={topics.activity} />
    <TopicRow label="Commande" mode="Publication" value={topics.command} />
  </div>;
}

function TopicRow({ label, mode, retained = false, value }: { label: string; mode: string; retained?: boolean; value: string }) {
  return <div className="flex flex-col gap-2 border-b border-border p-3 last:border-b-0 sm:flex-row sm:items-center">
    <div className="flex min-w-36 items-center gap-2"><strong className="text-sm">{label}</strong><StatusBadge tone={mode === 'Publication' ? 'blue' : 'green'}>{mode}</StatusBadge></div>
    <code className="min-w-0 flex-1 break-all rounded bg-muted px-2 py-1 text-xs">{value}</code>
    <StatusBadge title={retained ? 'Le serveur conserve la dernière valeur.' : 'Ce message est instantané et n’est pas conservé.'}>{retained ? 'Conservé' : 'Instantané'}</StatusBadge>
    <CopyButton value={value} />
  </div>;
}
