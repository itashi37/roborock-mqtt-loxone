import { RotateCcw, Save } from 'lucide-react';
import type { LoxoneExportSelection, LoxoneRoom } from '@/types/loxone';
import type { RoomDraftValidation } from '@/lib/loxone';
import { EmptyState, Spinner, StatusBadge } from './LoxoneUI';

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
  if (rooms.length === 0) return <EmptyState>No commandable segment was returned by <code>get_room_mapping</code>.</EmptyState>;
  return <div className="overflow-hidden rounded-xl border border-border">
    <div className="hidden grid-cols-[54px_90px_1fr_1.2fr_110px_130px] gap-3 bg-muted/60 px-3 py-2 text-xs font-medium uppercase tracking-wide text-muted-foreground md:grid">
      <span>Export</span><span>Segment</span><span>Roborock name</span><span>Loxone name</span><span>Status</span><span className="text-right">Action</span>
    </div>
    <div className="divide-y divide-border">
      {rooms.map(room => {
        const state = validation[room.id];
        const key = `room:${slug}:${room.id}`;
        const storedValue = room.override_name ?? room.effective_name;
        const currentValue = drafts[String(room.id)] ?? storedValue;
        const unchanged = currentValue.trim() === storedValue.trim();
        return <div key={room.id} className={`grid gap-3 p-3 md:grid-cols-[54px_90px_1fr_1.2fr_110px_130px] md:items-start ${state?.error ? 'bg-red-500/5' : 'bg-background'}`}>
          <label className="flex items-center gap-2 text-xs text-muted-foreground md:block">
            <span className="md:hidden">Include in export</span>
            <input type="checkbox" checked={Boolean(selected?.room_ids.includes(room.id))} disabled={!selected} onChange={event => onSelect(room.id, event.target.checked)} className="h-4 w-4" />
          </label>
          <div><span className="text-xs text-muted-foreground md:hidden">Segment ID · </span><strong className="tabular-nums">{room.id}</strong></div>
          <div className="min-w-0">
            <span className="text-xs text-muted-foreground md:hidden">Roborock · </span><span>{room.roborock_name}</span>
            <div className="mt-1"><StatusBadge tone="green" title="This segment is present in get_room_mapping and can be sent to segment_clean.">Commandable</StatusBadge></div>
          </div>
          <div className="min-w-0">
            <label htmlFor={`room-${room.id}`} className="mb-1 block text-xs text-muted-foreground md:hidden">Loxone name</label>
            <input id={`room-${room.id}`} value={currentValue} maxLength={81} onChange={event => onDraft(room.id, event.target.value)} className={`w-full rounded-lg border bg-card px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring ${state?.error ? 'border-red-500' : 'border-border'}`} aria-invalid={Boolean(state?.error)} aria-describedby={state?.error ? `room-${room.id}-error` : undefined} />
            {state?.error && <p id={`room-${room.id}-error`} className="mt-1 text-xs text-red-500">{state.error}</p>}
          </div>
          <div>{state?.error ? <StatusBadge tone="red">Conflict</StatusBadge> : <StatusBadge tone="green">Valid</StatusBadge>}</div>
          <div className="flex justify-end gap-2">
            <button type="button" onClick={() => onSave(room.id)} disabled={busy.has(key) || unchanged || Boolean(state?.error)} className="touch-target inline-flex items-center gap-1 rounded-lg border border-border px-2 text-sm hover:bg-accent disabled:opacity-40" title="Save this room name">
              {busy.has(key) ? <Spinner label="" /> : <Save className="h-4 w-4" />} Save
            </button>
            <button type="button" onClick={() => onReset(room.id)} disabled={busy.has(key) || !room.override_name} className="touch-target inline-flex items-center justify-center rounded-lg border border-border px-2 text-muted-foreground hover:bg-accent disabled:opacity-40" title="Reset to the Roborock or configured name" aria-label={`Reset room ${room.id}`}>
              <RotateCcw className="h-4 w-4" />
            </button>
          </div>
        </div>;
      })}
    </div>
  </div>;
}
