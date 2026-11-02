import { MapPin, RotateCcw, Save } from 'lucide-react';
import type { LoxoneExportSelection, LoxoneRoom } from '@/types/loxone';
import type { RoomDraftValidation } from '@/lib/loxone';
import { EmptyState, Spinner } from './LoxoneUI';

interface Props {
  slug: string;
  rooms: LoxoneRoom[];
  selected?: LoxoneExportSelection;
  drafts: Record<string, string>;
  validation: Record<number, RoomDraftValidation>;
  busy: Set<string>;
  onDraft: (roomID: number, value: string) => void;
  onSave: (roomID: number) => void;
  onReset: (roomID: number) => void;
  onSelect: (roomID: number, selected: boolean) => void;
}

export function RoomMappingTable({ slug, rooms, selected, drafts, validation, busy, onDraft, onSave, onReset, onSelect }: Props) {
  if (!rooms.length) return <EmptyState>Aucune pièce n’a encore été reçue. Vérifiez que la carte de ce robot contient des pièces définies dans l’application Roborock.</EmptyState>;
  return <div className="overflow-hidden rounded-xl border border-slate-200"><div className="hidden grid-cols-[30px_1fr_1.1fr_130px] gap-3 bg-slate-50 px-4 py-3 text-[10px] font-medium uppercase tracking-wider text-slate-400 md:grid"><span title="Inclure dans le dossier">ZIP</span><span>Pièce Roborock</span><span>Nom personnalisé</span><span /></div><div className="divide-y divide-slate-100">{rooms.map(room => {
    const state = validation[room.id];
    const key = `room:${slug}:${room.id}`;
    const storedValue = room.override_name ?? room.effective_name;
    const currentValue = drafts[String(room.id)] ?? storedValue;
    const unchanged = currentValue.trim() === storedValue.trim();
    const inputID = `room-${slug}-${room.id}`;
    const error = state?.error ? !state.value ? 'Le nom de la pièce est obligatoire.' : state.conflict ? 'Ce nom est déjà utilisé par une autre pièce.' : 'Le nom ne peut pas dépasser 80 caractères.' : undefined;
    return <div key={room.id} className={`grid gap-3 px-4 py-4 md:grid-cols-[30px_1fr_1.1fr_130px] md:items-center ${error ? 'bg-rose-50/50' : 'bg-white'}`}><label className="flex items-center gap-2 text-xs text-slate-400"><input type="checkbox" checked={Boolean(selected?.room_ids.includes(room.id))} disabled={!selected} onChange={event => onSelect(room.id, event.target.checked)} className="h-4 w-4 accent-emerald-600" aria-label={`Inclure ${room.effective_name} dans le dossier`} /><span className="md:hidden">Inclure dans le dossier</span></label><div className="flex min-w-0 items-center gap-2.5"><MapPin className="h-4 w-4 shrink-0 text-slate-300" /><div className="min-w-0"><strong className="block truncate text-sm font-medium text-slate-700">{room.roborock_name || room.effective_name}</strong><span className="mt-1 block text-[10px] text-slate-400">Identifiant {room.id}</span></div></div><div><label className="sr-only" htmlFor={inputID}>Nom de la pièce {room.id}</label><input id={inputID} value={currentValue} maxLength={81} onChange={event => onDraft(room.id, event.target.value)} className={`min-h-10 w-full rounded-xl border bg-white px-3 text-xs text-slate-700 outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-50 ${error ? 'border-rose-300' : 'border-slate-200'}`} aria-invalid={Boolean(error)} aria-describedby={error ? `${inputID}-error` : undefined} />{error && <p id={`${inputID}-error`} className="mt-1 text-[10px] text-rose-600">{error}</p>}</div><div className="flex justify-end gap-1.5"><button type="button" onClick={() => onSave(room.id)} disabled={busy.has(key) || unchanged || Boolean(error)} className="inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-slate-200 px-2.5 text-[11px] font-medium text-slate-600 hover:bg-emerald-50 disabled:opacity-35" title="Enregistrer ce nom">{busy.has(key) ? <Spinner label="" /> : <Save className="h-3.5 w-3.5" />}Enregistrer</button><button type="button" onClick={() => onReset(room.id)} disabled={busy.has(key) || !room.override_name} className="inline-flex min-h-9 min-w-9 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-50 disabled:opacity-30" title="Rétablir le nom d’origine" aria-label={`Rétablir le nom de la pièce ${room.id}`}><RotateCcw className="h-3.5 w-3.5" /></button></div></div>;
  })}</div></div>;
}
