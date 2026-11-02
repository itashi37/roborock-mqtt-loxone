import { ChevronDown, Download, FileArchive } from 'lucide-react';
import type { LoxoneExportSelection, LoxoneRobot } from '@/types/loxone';
import { Spinner, StatusBadge } from './LoxoneUI';

interface Props {
  robots: LoxoneRobot[];
  selection: LoxoneExportSelection[];
  mqttRobotCount: number;
  templateStatus: { native_generation: boolean; format_verified: boolean; reason: string; required_samples: string[] };
  perRobot: number;
  limit: number;
  required: number;
  exceeds: boolean;
  exporting: boolean;
  onRobotSelected: (slug: string, selected: boolean) => void;
  onDownload: () => void;
}

export function LoxoneExportPanel({ robots, selection, mqttRobotCount, templateStatus, perRobot, limit, required, exceeds, exporting, onRobotSelected, onDownload }: Props) {
  return <section className="rounded-2xl border border-slate-200 bg-white p-5 sm:p-6" aria-labelledby="loxone-export-heading">
    <div className="flex flex-col gap-5 sm:flex-row sm:items-center"><div className="flex items-start gap-3.5"><span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-slate-50 text-slate-500"><FileArchive className="h-5 w-5" strokeWidth={1.5} /></span><div><h2 id="loxone-export-heading" className="text-sm font-semibold text-slate-800">Votre dossier de configuration</h2><p className="mt-1 max-w-lg text-xs leading-relaxed text-slate-400">Téléchargez les noms d’entrées et les commandes de vos robots pour les garder à portée de main.</p></div></div><button onClick={onDownload} disabled={exporting || !selection.length} className="inline-flex min-h-10 shrink-0 items-center justify-center gap-2 self-start rounded-xl border border-slate-200 bg-white px-4 text-xs font-medium text-slate-600 transition hover:border-emerald-300 hover:bg-emerald-50 disabled:opacity-50 sm:ml-auto sm:self-auto">{exporting ? <Spinner label="Préparation…" /> : <><Download className="h-3.5 w-3.5" />Télécharger le dossier</>}</button></div>
    <details className="group mt-4 border-t border-slate-100 pt-4"><summary className="flex cursor-pointer list-none items-center gap-2 text-xs text-slate-500">{selection.length} robot{selection.length > 1 ? 's' : ''} sélectionné{selection.length > 1 ? 's' : ''}<span className="text-slate-300">·</span>Personnaliser le téléchargement<ChevronDown className="ml-auto h-3.5 w-3.5 text-slate-400 transition-transform group-open:rotate-180" /></summary><div className="mt-4 space-y-4"><div className="grid gap-2 sm:grid-cols-2">{robots.map(robot => {
      const item = selection.find(selected => selected.slug === robot.slug);
      return <label key={robot.slug} className={`flex cursor-pointer items-center gap-3 rounded-xl border p-3 ${item ? 'border-emerald-100 bg-emerald-50/50' : 'border-slate-200'}`}><input type="checkbox" checked={Boolean(item)} onChange={event => onRobotSelected(robot.slug, event.target.checked)} className="h-4 w-4 accent-emerald-600" /><span className="min-w-0 flex-1"><strong className="block truncate text-xs font-medium text-slate-700">{robot.name}</strong><span className="mt-1 block text-[10px] text-slate-400">{item?.room_ids.length ?? 0} pièces · {item?.scene_ids.length ?? 0} programmes</span></span></label>;
    })}</div><p className="text-xs leading-relaxed text-slate-400">Ajustez les pièces et programmes dans l’onglet correspondant du robot. Le dossier est un guide de configuration ; il ne contient aucun mot de passe et ne modifie pas votre Miniserver.</p>{!templateStatus.native_generation && <p className="text-xs leading-relaxed text-slate-400">L’import natif de modèles Loxone Config n’est pas encore disponible. Les adresses et instructions restent prêtes à copier.</p>}{mqttRobotCount > 0 && <details className="rounded-xl border border-slate-200 p-3"><summary className="cursor-pointer text-xs text-slate-500">Informations sur la connexion MQTT existante</summary><div className="mt-3 flex items-center gap-2"><StatusBadge tone={exceeds ? 'amber' : 'green'}>{required} / {limit} abonnements</StatusBadge><span className="text-xs text-slate-400">{mqttRobotCount} robots × {perRobot} abonnements</span></div>{exceeds && <p className="mt-2 text-xs text-amber-700">La sélection dépasse la limite de {limit} abonnements. Le téléchargement reste disponible.</p>}</details>}</div></details>
  </section>;
}
