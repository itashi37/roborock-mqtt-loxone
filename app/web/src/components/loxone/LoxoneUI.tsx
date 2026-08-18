import { Check, Clipboard, Loader2 } from 'lucide-react';
import { useEffect, useState } from 'react';

export type Tone = 'green' | 'red' | 'amber' | 'blue' | 'neutral';

const tones: Record<Tone, string> = {
  green: 'border-green-500/30 bg-green-500/10 text-green-600 dark:text-green-400',
  red: 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400',
  amber: 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  blue: 'border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400',
  neutral: 'border-border bg-muted text-muted-foreground',
};

export function StatusBadge({ children, tone = 'neutral', title }: { children: React.ReactNode; tone?: Tone; title?: string }) {
  return <span title={title} className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${tones[tone]}`}>{children}</span>;
}

export function BatteryGauge({ value }: { value: number }) {
  const bounded = Math.max(0, Math.min(100, value || 0));
  const color = bounded <= 15 ? 'bg-red-500' : bounded <= 35 ? 'bg-amber-500' : 'bg-green-500';
  return <div className="flex items-center gap-2" title={`Battery: ${bounded}%`}>
    <div className="h-2.5 w-24 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label="Battery" aria-valuemin={0} aria-valuemax={100} aria-valuenow={bounded}>
      <div className={`h-full rounded-full transition-all ${color}`} style={{ width: `${bounded}%` }} />
    </div>
    <strong className="tabular-nums">{bounded}%</strong>
  </div>;
}

export function CopyButton({ value, label = 'Copy value', disabled = false }: { value: string; label?: string; disabled?: boolean }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };
  return <button type="button" onClick={() => void copy()} disabled={disabled} className="touch-target inline-flex items-center justify-center rounded-lg border border-border p-2 hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40" title={label} aria-label={label}>
    {copied ? <Check className="h-4 w-4 text-green-500" /> : <Clipboard className="h-4 w-4" />}
  </button>;
}

export function Spinner({ label = 'Loading' }: { label?: string }) {
  return <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{label}</span>;
}

export function EmptyState({ children }: { children: React.ReactNode }) {
  return <div className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">{children}</div>;
}

export interface ToastMessage { id: number; kind: 'success' | 'error'; text: string }

export function ToastRegion({ toast, dismiss }: { toast: ToastMessage | null; dismiss: () => void }) {
  useEffect(() => {
    if (!toast) return;
    const timeout = window.setTimeout(dismiss, 4500);
    return () => window.clearTimeout(timeout);
  }, [toast, dismiss]);
  if (!toast) return null;
  return <div className="fixed bottom-4 right-4 z-50 max-w-sm" role="status" aria-live="polite">
    <button onClick={dismiss} className={`w-full rounded-xl border p-4 text-left text-sm shadow-lg backdrop-blur ${toast.kind === 'success' ? tones.green : tones.red}`}>
      {toast.text}
    </button>
  </div>;
}

export function formatTimestamp(timestamp: number): string {
  if (!timestamp) return 'Never';
  return new Date(timestamp * 1000).toLocaleString();
}

export function relativeTimestamp(timestamp: number): string {
  if (!timestamp) return 'Never';
  const seconds = Math.max(0, Math.round(Date.now() / 1000 - timestamp));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}
