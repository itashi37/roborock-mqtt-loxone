import { Check, Copy, Loader2 } from 'lucide-react';
import { useEffect, useState } from 'react';

export type Tone = 'green' | 'red' | 'amber' | 'blue' | 'neutral';

const tones: Record<Tone, string> = {
  green: 'border-green-200 bg-green-50 text-green-700',
  red: 'border-rose-100 bg-rose-50 text-rose-700',
  amber: 'border-amber-100 bg-amber-50 text-amber-700',
  blue: 'border-sky-100 bg-sky-50 text-sky-700',
  neutral: 'border-slate-200 bg-slate-50 text-slate-500',
};

export function StatusBadge({ children, tone = 'neutral', title }: { children: React.ReactNode; tone?: Tone; title?: string }) {
  return <span title={title} className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-1 text-[11px] font-medium ${tones[tone]}`}>{children}</span>;
}

export function BatteryGauge({ value }: { value: number }) {
  const bounded = Math.max(0, Math.min(100, value || 0));
  const color = bounded <= 15 ? 'bg-red-500' : bounded <= 35 ? 'bg-amber-500' : 'bg-green-500';
  return <div className="flex items-center gap-2" title={`Batterie : ${bounded} %`}>
    <div className="h-1.5 w-16 overflow-hidden rounded-full bg-slate-100" role="progressbar" aria-label="Batterie" aria-valuemin={0} aria-valuemax={100} aria-valuenow={bounded}>
      <div className={`h-full rounded-full transition-all ${color}`} style={{ width: `${bounded}%` }} />
    </div>
    <strong className="tabular-nums">{bounded}%</strong>
  </div>;
}

export function CopyButton({ value, label = 'Copier', disabled = false }: { value: string; label?: string; disabled?: boolean }) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  const copy = async () => {
    try {
      if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(value);
      else {
        const field = document.createElement('textarea');
        field.value = value;
        field.style.position = 'fixed';
        field.style.opacity = '0';
        document.body.appendChild(field);
        field.select();
        const success = document.execCommand('copy');
        field.remove();
        if (!success) throw new Error('Copie indisponible');
      }
      setCopied(true); setFailed(false);
      window.setTimeout(() => setCopied(false), 1600);
    } catch { setFailed(true); }
  };
  return <button type="button" onClick={() => void copy()} disabled={disabled} className={`inline-flex min-h-9 min-w-9 shrink-0 items-center justify-center rounded-lg border p-2 transition hover:border-blue-300 hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-40 ${failed ? 'border-rose-200 text-rose-600' : 'border-slate-200 bg-white text-slate-500'}`} title={failed ? 'Sélectionnez le texte pour le copier manuellement' : copied ? 'Copié !' : label} aria-label={copied ? 'Copié' : label}>
    {copied ? <Check className="h-3.5 w-3.5 text-blue-600" /> : <Copy className="h-3.5 w-3.5" />}
  </button>;
}

export function Spinner({ label = 'Chargement…' }: { label?: string }) {
  return <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{label}</span>;
}

export function EmptyState({ children }: { children: React.ReactNode }) {
  return <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/50 p-8 text-center text-sm leading-relaxed text-slate-500">{children}</div>;
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
  if (!timestamp) return 'Pas encore reçue';
  return new Date(timestamp * 1000).toLocaleString('fr-FR');
}

export function relativeTimestamp(timestamp: number): string {
  if (!timestamp) return 'Pas encore reçue';
  const seconds = Math.max(0, Math.round(Date.now() / 1000 - timestamp));
  if (seconds < 10) return 'À l’instant';
  if (seconds < 60) return `Il y a ${seconds} s`;
  if (seconds < 3600) return `Il y a ${Math.floor(seconds / 60)} min`;
  if (seconds < 86400) return `Il y a ${Math.floor(seconds / 3600)} h`;
  return `Il y a ${Math.floor(seconds / 86400)} j`;
}

const stateLabels: Record<string, string> = {
  unknown: 'État inconnu', offline: 'Hors ligne', idle: 'Au repos', charging: 'En charge', docked: 'À la station',
  cleaning: 'Nettoyage en cours', segment_cleaning: 'Nettoyage d’une pièce', room_cleaning: 'Nettoyage d’une pièce',
  spot_cleaning: 'Nettoyage localisé', zoned_cleaning: 'Nettoyage d’une zone', paused: 'En pause', returning_home: 'Retour à la station',
  returning: 'Retour à la station', washing_mop: 'Lavage des serpillières', drying_mop: 'Séchage des serpillières',
  servicing_dock: 'Entretien à la station', emptying_dustbin: 'Vidage du bac', error: 'Intervention nécessaire', stuck: 'Robot bloqué',
  full: 'Nettoyage complet', room: 'Nettoyage par pièce', scene: 'Programme Roborock',
  accepted: 'Acceptée', queued: 'En attente', sent: 'Envoyée', running: 'En cours', completed: 'Terminée', failed: 'Échec',
  cleaning_started: 'Nettoyage démarré', cleaning_completed: 'Nettoyage terminé', cleaning_finished: 'Nettoyage terminé',
  cleaning_paused: 'Nettoyage en pause', room_entered: 'Entrée dans une pièce', online: 'Connexion rétablie',
  start: 'Démarrer', pause: 'Pause', dock: 'Retour à la station', stop: 'Arrêter',
  available: 'Disponible', emptying: 'Vidage en cours', washing: 'Lavage en cours', drying: 'Séchage en cours',
};

export function displayRobotState(value: string): string {
  return stateLabels[value] ?? (value || 'État inconnu').replace(/_/g, ' ');
}

export function displayActivity(activity: { type: string; command?: string; state?: string; error?: string; event?: string; room_name?: string; error_text?: string }): string {
  if (activity.type === 'command') return `${displayRobotState(activity.command ?? 'Commande')}${activity.state ? ` · ${displayRobotState(activity.state)}` : ''}${activity.error ? ` · ${activity.error}` : ''}`;
  return `${displayRobotState(activity.event ?? 'Événement')}${activity.room_name ? ` · ${activity.room_name}` : ''}${activity.error_text ? ` · ${activity.error_text}` : ''}`;
}
