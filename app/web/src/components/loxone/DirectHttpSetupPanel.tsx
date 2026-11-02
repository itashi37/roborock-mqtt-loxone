import { ArrowDownLeft, ArrowUpRight, Check, ChevronDown, KeyRound, Link2, Search, ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { directCommandURL, directConnectorAddress, normalizeBridgeAddress, suggestedLoxoneInputLabel } from '@/lib/loxone';
import type { LoxoneDirectInput, LoxoneDirectOutput, LoxoneRobot } from '@/types/loxone';
import { CopyButton, EmptyState, StatusBadge } from './LoxoneUI';

interface Props { robot: LoxoneRobot; apiUsername: string; tokenConfigured: boolean }
type Step = 'connection' | 'inputs' | 'commands';

const inputDescriptions: Record<string, string> = {
  online: 'Connexion au service Roborock', robot_online: 'Le robot répond à la passerelle', running: 'Reste actif pendant toute la mission, y compris les passages intermédiaires à la station',
  battery: 'Niveau de batterie en %', state: 'État du robot sous forme de code', state_text: 'État du robot en toutes lettres',
  current_room_id: 'Identifiant de la pièce actuellement nettoyée', current_room_name: 'Nom de la pièce actuelle',
  active_program: 'Type de nettoyage en cours', active_scene_id: 'Identifiant du programme lancé par la passerelle', active_scene_name: 'Nom du programme actif',
  error_code: 'Code d’erreur du robot', error_text: 'Détail de l’erreur du robot', clean_area: 'Surface nettoyée en m²', clean_time_seconds: 'Durée du nettoyage en secondes',
  last_seen: 'Date de dernière réception, au format Unix', main_brush: 'Durée restante de la brosse principale', side_brush: 'Durée restante de la brosse latérale',
  filter: 'Durée restante du filtre', sensor: 'Durée restante avant entretien des capteurs', dock_type: 'Type de station', charge_status: 'État de la charge',
  dock_error_status: 'Code d’erreur signalé par la station', dust_collection_status: 'État du vidage automatique', wash_status: 'État du lavage des serpillières', dry_status: 'État du séchage des serpillières',
};
const essentialFields = new Set(['running', 'online', 'battery', 'state_text', 'current_room_id', 'current_room_name', 'error_code', 'error_text', 'dock_error_status']);
const kindLabels = { digital: 'Tout ou rien', analog: 'Numérique', text: 'Texte' };

export function DirectHttpSetupPanel({ robot, apiUsername, tokenConfigured }: Props) {
  const [step, setStep] = useState<Step>('inputs');
  const [query, setQuery] = useState('');
  const [showAll, setShowAll] = useState(false);
  const [bridgeAddress, setBridgeAddress] = useState(() => typeof window === 'undefined' ? 'http://IP-DU-SYNOLOGY:8080' : window.location.origin);
  const [token, setToken] = useState('');
  const address = useMemo(() => {
    try { return { value: normalizeBridgeAddress(bridgeAddress), error: '' }; }
    catch { return { value: '', error: 'Saisissez une adresse HTTP ou HTTPS valide.' }; }
  }, [bridgeAddress]);
  const tokenReady = token.length >= 32;
  const connectorAddress = address.value ? directConnectorAddress(address.value, apiUsername, tokenReady ? token : '') : '';
  const connectorPreview = connectorAddress && !tokenReady ? directConnectorAddress(address.value, apiUsername, 'VOTRE_JETON_API') : connectorAddress;
  const inputs = robot.direct_inputs ?? [];
  const outputs = robot.direct_outputs ?? [];
  const visibleInputs = inputs.filter(input => (showAll || query || essentialFields.has(input.field)) && `${input.name} ${input.field} ${inputDescriptions[input.field] ?? ''} ${suggestedLoxoneInputLabel(robot.name, input.field)}`.toLocaleLowerCase('fr').includes(query.toLocaleLowerCase('fr')));
  const standard = outputs.filter(output => ['start', 'pause', 'dock'].includes(output.command));
  const rooms = outputs.filter(output => output.path.includes('/rooms/'));
  const scenes = outputs.filter(output => output.path.includes('/scenes/'));
  const advanced = outputs.filter(output => !standard.includes(output) && !rooms.includes(output) && !scenes.includes(output));

  if (!robot.direct_enabled) return <div className="rounded-2xl border border-slate-200 bg-slate-50 p-6">
    <Link2 className="mb-3 h-6 w-6 text-blue-700" />
    <h3 className="font-semibold text-slate-800">La connexion directe n’est pas activée pour ce robot</h3>
    <p className="mt-2 max-w-xl text-sm leading-relaxed text-slate-500">Activez la connexion Loxone dans les réglages pour retrouver les noms d’entrées et les commandes à copier.</p>
    <Link to="/setup" className="mt-4 inline-flex min-h-10 items-center rounded-xl bg-blue-700 px-4 text-sm font-medium text-white hover:bg-blue-800">Ouvrir les réglages</Link>
  </div>;

  return <div className="space-y-6">
    <div className="flex items-start gap-3 rounded-xl border border-blue-100 bg-blue-50/60 px-4 py-3.5">
      <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-blue-700" />
      <div><p className="text-sm font-medium text-blue-900">Vos adresses Loxone sont conservées</p><p className="mt-0.5 text-xs leading-relaxed text-blue-800/75">Retrouvez ici les noms exacts et les commandes de ce robot. Aucune modification de votre configuration existante n’est nécessaire.</p></div>
    </div>
    <div className="flex flex-col gap-1 rounded-xl bg-slate-100 p-1 sm:flex-row" role="tablist" aria-label="Configuration Loxone">
      {([{ id: 'connection', number: '1', title: 'Connexion', icon: Link2 }, { id: 'inputs', number: '2', title: 'Entrées Loxone', icon: ArrowDownLeft }, { id: 'commands', number: '3', title: 'Commandes', icon: ArrowUpRight }] as const).map(item => <button key={item.id} type="button" role="tab" aria-selected={step === item.id} aria-controls={`loxone-step-${robot.slug}`} onClick={() => setStep(item.id)} className={`flex min-h-11 flex-1 items-center justify-center gap-2 rounded-lg px-3 text-sm font-medium transition ${step === item.id ? 'bg-white text-blue-800 shadow-sm' : 'text-slate-500 hover:bg-white/60 hover:text-slate-800'}`}><span className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] ${step === item.id ? 'bg-blue-100 text-blue-800' : 'bg-slate-200/70 text-slate-500'}`}>{item.number}</span>{item.title}</button>)}
    </div>
    <div id={`loxone-step-${robot.slug}`} role="tabpanel">
      {step === 'connection' && <div className="space-y-5">
        <SectionIntro title="Un connecteur pour piloter ce robot" description="Dans Loxone Config, créez une sortie virtuelle avec l’adresse ci-dessous. Les entrées sont mises à jour automatiquement par la passerelle." />
        <div className="grid gap-4 md:grid-cols-2">
          <label className="block text-sm font-medium text-slate-700">Adresse de la passerelle<input value={bridgeAddress} onChange={event => setBridgeAddress(event.target.value)} className="mt-2 min-h-11 w-full rounded-xl border border-slate-200 bg-white px-3 font-mono text-xs outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-100" placeholder="http://192.168.1.20:8080" /><span className={`mt-2 block text-xs font-normal ${address.error ? 'text-rose-600' : 'text-slate-400'}`}>{address.error || 'L’adresse de votre Synology, accessible depuis le Miniserver.'}</span></label>
          <label className="block text-sm font-medium text-slate-700">Jeton de commande<input type="password" autoComplete="off" value={token} onChange={event => setToken(event.target.value.trim())} className="mt-2 min-h-11 w-full rounded-xl border border-slate-200 bg-white px-3 font-mono text-xs outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-100" placeholder="Collez votre jeton pour compléter l’adresse" /><span className="mt-2 flex items-center gap-1.5 text-xs font-normal text-slate-400">{tokenConfigured ? <><Check className="h-3 w-3 text-blue-600" />Un jeton est déjà configuré.</> : 'Aucun jeton n’est encore configuré.'} Il reste dans cet onglet.</span></label>
        </div>
        <CopyField label="Adresse du connecteur de sortie virtuelle" value={connectorPreview} copyEnabled={tokenReady && !address.error} />
        <details className="rounded-xl border border-slate-200 px-4 py-3"><summary className="cursor-pointer text-xs font-medium text-slate-600">J’ai déjà une connexion fonctionnelle ou j’ai perdu mon jeton</summary><p className="mt-3 text-xs leading-relaxed text-slate-500">Si vos commandes fonctionnent déjà dans Loxone, conservez votre connecteur actuel. Le jeton n’est nécessaire ici que pour reconstruire son adresse. Si vous le remplacez dans les <Link to="/setup" className="font-medium text-blue-700 underline">réglages</Link>, vous devrez aussi le remplacer dans Loxone Config.</p></details>
        <div className="flex items-center gap-2 text-xs text-slate-400"><KeyRound className="h-3.5 w-3.5" />Utilisateur : <code className="text-slate-600">{apiUsername}</code> · authentification Basic</div>
      </div>}
      {step === 'inputs' && <div className="space-y-5">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between"><SectionIntro title="Les informations reçues dans Loxone" description="Copiez le nom exact dans votre entrée virtuelle. Les libellés ci-dessous vous aident à identifier chaque information." /><StatusBadge tone="green">{inputs.length} entrées disponibles</StatusBadge></div>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center"><label className="relative flex-1"><Search className="absolute left-3 top-3 h-4 w-4 text-slate-400" /><input aria-label="Rechercher une entrée Loxone" value={query} onChange={event => setQuery(event.target.value)} placeholder="Rechercher une information…" className="min-h-10 w-full rounded-xl border border-slate-200 bg-slate-50/50 pl-9 pr-3 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-50" /></label><button type="button" onClick={() => setShowAll(value => !value)} className={`min-h-10 rounded-xl border px-3 text-xs font-medium transition ${showAll ? 'border-blue-200 bg-blue-50 text-blue-800' : 'border-slate-200 text-slate-500 hover:bg-slate-50'}`}>{showAll ? 'Afficher les essentielles' : 'Afficher toutes les entrées'}</button></div>
        {visibleInputs.length === 0 ? <EmptyState>{inputs.length ? 'Aucune entrée ne correspond à votre recherche.' : 'Les entrées apparaîtront à la prochaine réception de l’état du robot.'}</EmptyState> : <div className="overflow-hidden rounded-xl border border-slate-200"><div className="hidden grid-cols-[1fr_110px_1.3fr] gap-4 bg-slate-50 px-4 py-3 text-[10px] font-semibold uppercase tracking-wider text-slate-400 md:grid"><span>Information</span><span>Type d’entrée</span><span>Nom exact à conserver</span></div><div className="divide-y divide-slate-100">{visibleInputs.map(input => <InputRow key={`${input.field}:${input.name}`} input={input} robotName={robot.name} />)}</div></div>}
        <p className="text-xs leading-relaxed text-slate-400">Les noms affichés sont ceux fournis par votre configuration actuelle. Pour les entrées de type « Texte », utilisez une entrée texte virtuelle dans Loxone Config.</p>
      </div>}
      {step === 'commands' && <div className="space-y-5">
        <SectionIntro title="Les actions envoyées par Loxone" description="Ajoutez une commande sous votre connecteur de sortie virtuelle. Copiez le chemin dans « Commande lorsque ON » et choisissez la méthode POST." />
        <CommandGroup title="Les commandes essentielles" outputs={standard} connector={connectorAddress} ready={tokenReady && !address.error} open />
        <CommandGroup title={`Nettoyer une pièce · ${rooms.length}`} outputs={rooms} connector={connectorAddress} ready={tokenReady && !address.error} />
        <CommandGroup title={`Lancer un programme · ${scenes.length}`} outputs={scenes} connector={connectorAddress} ready={tokenReady && !address.error} />
        <CommandGroup title={`Autres commandes disponibles · ${advanced.length}`} outputs={advanced} connector={connectorAddress} ready={tokenReady && !address.error} />
        {outputs.length === 0 && <EmptyState>Aucune commande n’est encore disponible pour ce robot.</EmptyState>}
      </div>}
    </div>
  </div>;
}

function SectionIntro({ title, description }: { title: string; description: string }) {
  return <div><h3 className="text-base font-semibold tracking-tight text-slate-800">{title}</h3><p className="mt-1.5 max-w-2xl text-sm leading-relaxed text-slate-500">{description}</p></div>;
}

function CopyField({ label, value, copyEnabled }: { label: string; value: string; copyEnabled: boolean }) {
  return <div><p className="mb-2 text-xs font-medium text-slate-500">{label}</p><div className="flex min-w-0 items-center gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-2 pl-3"><code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap text-xs text-slate-700">{value || 'Saisissez l’adresse de la passerelle'}</code><CopyButton value={value} disabled={!copyEnabled} label={`Copier : ${label}`} /></div></div>;
}

function InputRow({ input, robotName }: { input: LoxoneDirectInput; robotName: string }) {
  const label = suggestedLoxoneInputLabel(robotName, input.field).replace(`${robotName.replace(/^roborock\s+/i, '').trim()} — `, '');
  return <div className="grid items-center gap-3 bg-white px-4 py-4 md:grid-cols-[1fr_110px_1.3fr]">
    <div className="min-w-0"><p className="text-sm font-medium text-slate-700">{input.field === 'running' ? 'Robot en fonctionnement' : label}</p><p className="mt-1 text-[11px] leading-relaxed text-slate-400">{inputDescriptions[input.field] ?? input.field}</p></div>
    <div><StatusBadge tone={input.kind === 'digital' ? 'green' : input.kind === 'text' ? 'blue' : 'neutral'}>{kindLabels[input.kind]}</StatusBadge></div>
    <div className="flex min-w-0 items-center gap-2"><code className="min-w-0 flex-1 select-all break-all rounded-lg bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-600">{input.name}</code><CopyButton value={input.name} label={`Copier ${input.name}`} /></div>
  </div>;
}

function CommandGroup({ title, outputs, connector, ready, open = false }: { title: string; outputs: LoxoneDirectOutput[]; connector: string; ready: boolean; open?: boolean }) {
  if (!outputs.length) return null;
  const commandLabels: Record<string, string> = { start: 'Démarrer le nettoyage', pause: 'Mettre en pause', dock: 'Retourner à la station' };
  return <details open={open} className="group overflow-hidden rounded-xl border border-slate-200"><summary className="flex cursor-pointer list-none items-center justify-between gap-3 bg-slate-50/70 px-4 py-3.5 text-sm font-medium text-slate-700">{title}<ChevronDown className="h-4 w-4 text-slate-400 transition-transform group-open:rotate-180" /></summary><div className="divide-y divide-slate-100 border-t border-slate-100">{outputs.map(output => <div key={output.path} className="space-y-2.5 p-4"><div className="flex items-center gap-2"><strong className="text-sm font-medium text-slate-700">{commandLabels[output.command] ?? output.name}</strong><StatusBadge>POST</StatusBadge></div><div className="flex min-w-0 items-center gap-2"><code className="min-w-0 flex-1 select-all break-all rounded-lg bg-slate-50 px-3 py-3 text-xs text-slate-600">{output.path}</code><CopyButton value={output.path} label={`Copier la commande ${output.name}`} /></div>{ready && <details className="text-xs text-slate-400"><summary className="cursor-pointer">Adresse complète</summary><div className="mt-2"><CopyField label="URL complète avec authentification" value={directCommandURL(connector, output.path)} copyEnabled /></div></details>}</div>)}</div></details>;
}
