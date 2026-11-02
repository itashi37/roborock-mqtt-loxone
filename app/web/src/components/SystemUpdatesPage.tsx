import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Activity, AlertCircle, ArrowDownToLine, ArrowRight, Check, CheckCircle2, ChevronDown, Clock3, Database, ExternalLink, RefreshCw, Settings2, ShieldCheck, Wifi } from 'lucide-react';
import { checkForUpdates, fetchSystemStatus, fetchUpdateOperation, installUpdate, saveUpdateSettings } from '@/lib/api';
import type { SystemStatus, UpdateOperation, UpdateSettings } from '@/types/system';

const activeUpdateStages = new Set(['preparing', 'pulling', 'backing_up', 'restarting', 'validating', 'rollback']);
const primaryButton = 'inline-flex min-h-11 items-center justify-center gap-2 rounded-xl bg-[#007aff] px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-[#0071e3] disabled:cursor-not-allowed disabled:opacity-45';
const secondaryButton = 'inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-medium text-[#1d1d1f] transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-45';
const inputClass = 'mt-2 min-h-11 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-[#1d1d1f] outline-none focus:border-[#007aff] focus:ring-2 focus:ring-blue-100';

export function SystemUpdatesPage(_props: { returnSlug?: string }) {
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [channel, setChannel] = useState<'stable' | 'edge'>('stable');
  const [action, setAction] = useState<'check' | 'install' | 'settings' | null>(null);
  const [error, setError] = useState('');
  const [operation, setOperation] = useState<UpdateOperation | null>(null);
  const [updaterAvailable, setUpdaterAvailable] = useState<boolean | null>(null);
  const [confirmInstall, setConfirmInstall] = useState(false);
  const [settings, setSettings] = useState<UpdateSettings | null>(null);
  const [settingsSaved, setSettingsSaved] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const initialized = useRef(false);

  const load = async () => {
    const value = await fetchSystemStatus();
    setStatus(value);
    setSettings(current => current ?? value.update_settings);
    if (!initialized.current) {
      setChannel(value.channel === 'edge' ? 'edge' : 'stable');
      initialized.current = true;
    }
  };
  const loadOperation = async () => {
    try {
      const value = await fetchUpdateOperation();
      setOperation(value);
      setUpdaterAvailable(true);
      if (value.stage === 'success') await load();
    } catch {
      // The bridge can be briefly unreachable while the updater restarts it.
      setUpdaterAvailable(false);
    }
  };
  const refreshStatus = async () => {
    setRefreshing(true);
    setError('');
    try { await Promise.all([load(), loadOperation()]); }
    catch (reason) { setError(errorMessage(reason, 'Impossible de joindre la passerelle.')); }
    finally { setRefreshing(false); }
  };
  useEffect(() => { void refreshStatus(); }, []);
  useEffect(() => {
    if (!operation || !activeUpdateStages.has(operation.stage)) return;
    const timer = window.setInterval(() => void loadOperation(), 2000);
    return () => window.clearInterval(timer);
  }, [operation?.stage]);

  const check = async () => {
    setAction('check'); setError(''); setConfirmInstall(false);
    try {
      const update = await checkForUpdates(channel);
      setStatus(current => current ? { ...current, update } : current);
    } catch (reason) { setError(errorMessage(reason, 'La recherche de mise à jour a échoué.')); }
    finally { setAction(null); }
  };
  const install = async () => {
    setAction('install'); setError(''); setConfirmInstall(false);
    try {
      const next = await installUpdate(channel);
      setOperation(next); setUpdaterAvailable(true);
    } catch (reason) { setError(errorMessage(reason, 'Impossible de démarrer la mise à jour.')); }
    finally { setAction(null); }
  };
  const savePolicy = async () => {
    if (!settings) return;
    setAction('settings'); setError(''); setSettingsSaved(false);
    try {
      const saved = await saveUpdateSettings(settings);
      setSettings(saved);
      setStatus(current => current ? { ...current, update_settings: saved } : current);
      setSettingsSaved(true);
    } catch (reason) { setError(errorMessage(reason, 'Impossible d’enregistrer ces préférences.')); }
    finally { setAction(null); }
  };

  if (!status) return <section className="rounded-2xl border border-slate-200 bg-white p-10 text-center">
    {error ? <AlertCircle className="mx-auto h-8 w-8 text-amber-600" /> : <RefreshCw className="mx-auto h-8 w-8 animate-spin text-[#007aff]" />}
    <h1 className="mt-4 text-xl font-semibold text-[#1d1d1f]">{error ? 'Passerelle momentanément indisponible' : 'Connexion à la passerelle…'}</h1>
    <p className="mt-2 text-sm text-slate-500">{error || 'Nous récupérons votre version et l’état des mises à jour.'}</p>
    {error && <button onClick={() => void refreshStatus()} disabled={refreshing} className={`${primaryButton} mt-6`}>Réessayer</button>}
  </section>;

  const update = status.update;
  const operationActive = Boolean(operation && activeUpdateStages.has(operation.stage));
  const busy = action !== null || operationActive;
  const selectedUpdate = update.channel === channel;
  const checked = Boolean(update.checked_at && selectedUpdate);
  const available = checked && update.available;
  const healthy = status.health.status === 'healthy';
  const direct = status.transports.direct;
  const loxone = direct?.enabled ? direct : status.transports.mqtt;
  const failedDirectInputs = direct?.failed_inputs || [];
  const installedMode = status.update_settings.mode;

  return <div className="space-y-6 text-[#1d1d1f]">
    <header className="flex flex-wrap items-start justify-between gap-4">
      <div><p className="text-xs font-semibold uppercase tracking-[0.18em] text-[#007aff]">Votre passerelle</p><h1 className="mt-2 text-3xl font-semibold tracking-tight">Système & mises à jour</h1><p className="mt-2 text-sm text-slate-500">Une passerelle à jour, au moment que vous choisissez.</p></div>
      <button onClick={() => void refreshStatus()} disabled={refreshing || busy} className={secondaryButton}><RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />Actualiser</button>
    </header>
    {error && <div role="alert" className="flex items-start gap-3 rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-800"><AlertCircle className="mt-0.5 h-5 w-5 shrink-0" />{error}</div>}

    <div className="grid gap-5 lg:grid-cols-[0.85fr_1.4fr]">
      <section className="bridge-version-card relative overflow-hidden rounded-2xl p-7">
        <div className="absolute -right-12 -top-12 h-52 w-52 rounded-full border-[32px] border-black/[0.025]" aria-hidden="true" />
        <div className="relative flex h-full flex-col items-start">
          <div className="mb-9 flex h-12 w-12 items-center justify-center rounded-2xl bg-slate-100"><ShieldCheck className="h-6 w-6 text-primary" /></div>
          <p className="text-sm text-muted-foreground">Version installée</p><p className="mt-2 break-all text-4xl font-semibold tracking-tight">{status.version}</p>
          <span className="mt-3 rounded-full bg-slate-100 px-3 py-1 text-xs text-muted-foreground">{status.channel === 'edge' ? 'Canal de développement' : 'Version stable'}</span>
          <div className="mt-auto pt-9"><div className="flex items-center gap-2 text-sm font-medium"><span className={`h-2 w-2 rounded-full ${healthy ? 'bg-green-500' : 'bg-amber-300'}`} />{healthy ? 'Tout fonctionne normalement' : status.health.live ? 'Un point demande votre attention' : 'La passerelle rencontre un problème'}</div><p className="mt-2 text-xs text-muted-foreground">En service depuis {formatDuration(status.uptime_seconds)}</p></div>
        </div>
      </section>
      <section className="rounded-2xl border border-slate-200 bg-white p-6 sm:p-7">
        <div className="flex items-start justify-between gap-3"><div><h2 className="text-lg font-semibold">Mises à jour</h2><p className="mt-1 text-sm text-slate-500">{installedMode === 'automatic' ? 'L’installation automatique est activée dans vos préférences.' : 'Vous gardez la main sur chaque installation.'}</p></div><span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#f0f6ff] text-[#007aff]"><ArrowDownToLine className="h-5 w-5" /></span></div>
        <div className="mt-6 rounded-xl bg-[#f5f5f7] p-4"><div className="flex items-start gap-3">{action === 'check' ? <RefreshCw className="mt-0.5 h-5 w-5 shrink-0 animate-spin text-[#007aff]" /> : <CheckCircle2 className={`mt-0.5 h-5 w-5 shrink-0 ${available ? 'text-[#007aff]' : 'text-slate-400'}`} />}<div className="min-w-0"><p className="font-medium">{action === 'check' ? 'Recherche en cours…' : available ? `La version ${update.latest_version} est disponible` : checked ? 'Votre passerelle est à jour' : 'Prêt à rechercher une nouvelle version'}</p><p className="mt-1 text-xs leading-relaxed text-slate-500">{checked ? `Dernière vérification : ${formatDate(update.checked_at)}` : `Canal ${channel === 'stable' ? 'stable' : 'de développement'}. La recherche ne déclenche aucune installation.`}</p></div></div>{update.error && selectedUpdate && <p className="mt-3 text-sm text-amber-700">{update.error}</p>}</div>
        <div className="mt-5 flex flex-wrap gap-3"><button onClick={() => void check()} disabled={busy} className={available ? secondaryButton : primaryButton}><RefreshCw className={`h-4 w-4 ${action === 'check' ? 'animate-spin' : ''}`} />{action === 'check' ? 'Recherche…' : 'Rechercher une mise à jour'}</button>{available && <button onClick={() => setConfirmInstall(true)} disabled={busy || updaterAvailable !== true || !update.artifact_ready} className={primaryButton}><ArrowDownToLine className="h-4 w-4" />Installer {update.latest_version}</button>}</div>
        {available && !update.artifact_ready && <p className="mt-3 text-xs leading-relaxed text-amber-700">Cette version est en cours de publication. Relancez la recherche dans quelques minutes pour pouvoir l’installer.</p>}
        {updaterAvailable === false && !operationActive && <p className="mt-4 text-sm leading-relaxed text-amber-700">Le service de mise à jour est indisponible. Vérifiez que le conteneur <code className="break-all text-xs">roborock-mqtt-loxone-updater</code> fonctionne dans Container Manager.</p>}
        <div className="mt-6 flex items-start gap-2 border-t border-slate-100 pt-4 text-xs leading-relaxed text-slate-500"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-[#007aff]" /><span>Vos données sont sauvegardées avant l’installation. Si la nouvelle version échoue aux contrôles, la version précédente est restaurée.</span></div>
        {checked && (update.release_notes || update.release_url) && <details className="group mt-5 border-t border-slate-100 pt-4"><summary className="flex cursor-pointer list-none items-center justify-between text-sm font-medium">Les nouveautés<ChevronDown className="h-4 w-4 text-slate-400 transition group-open:rotate-180" /></summary>{update.release_notes && <pre className="mt-4 max-h-64 overflow-auto whitespace-pre-wrap text-sm leading-relaxed text-slate-600">{update.release_notes}</pre>}{update.release_url && <a href={update.release_url} target="_blank" rel="noreferrer" className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-[#007aff]">Voir la publication<ExternalLink className="h-3.5 w-3.5" /></a>}</details>}
      </section>
    </div>

    {confirmInstall && available && <section role="alertdialog" aria-labelledby="confirm-update-title" className="rounded-2xl border border-blue-200 bg-[#f0f6ff] p-6"><h2 id="confirm-update-title" className="text-lg font-semibold">Installer la version {update.latest_version} ?</h2><p className="mt-2 max-w-2xl text-sm leading-relaxed text-slate-600">La passerelle redémarrera pendant l’installation. Les échanges avec Loxone et les commandes des robots seront brièvement interrompus. Vos adresses Loxone et vos réglages sont conservés.</p><div className="mt-5 flex flex-wrap gap-3"><button onClick={() => void install()} disabled={busy || updaterAvailable !== true || !update.artifact_ready} className={primaryButton}><ArrowDownToLine className="h-4 w-4" />Confirmer l’installation</button><button onClick={() => setConfirmInstall(false)} className={secondaryButton}>Plus tard</button></div></section>}
    {operation && operation.stage !== 'idle' && <UpdateProgress operation={operation} reconnecting={operationActive && updaterAvailable === false} />}
    <section className="grid gap-3 sm:grid-cols-3" aria-label="État des services">
      <ServiceCard icon={<Wifi className="h-5 w-5" />} label="Connexion Loxone" value={!loxone?.enabled ? 'À configurer' : loxone.connected ? 'Connectée' : 'À vérifier'} good={Boolean(loxone?.enabled && loxone.connected)} detail={loxone?.enabled && loxone.connected ? 'Les échanges sont opérationnels.' : 'Consultez la page Loxone.'} />
      <ServiceCard icon={<RefreshCw className="h-5 w-5" />} label="Service de mise à jour" value={operationActive ? 'Installation en cours' : updaterAvailable === null ? 'Vérification…' : updaterAvailable ? 'Prêt' : 'Indisponible'} good={updaterAvailable === true} detail={updaterAvailable ? 'Disponible lorsque vous le décidez.' : 'Vérifiez son conteneur sur le NAS.'} />
      <ServiceCard icon={<Database className="h-5 w-5" />} label="Données & configuration" value={status.data_volume.writable ? 'Stockage disponible' : 'À vérifier'} good={status.data_volume.writable} detail={status.data_volume.writable ? `${formatBytes(status.data_volume.free_bytes)} d’espace libre` : status.data_volume.error || 'Impossible d’écrire dans le volume.'} />
    </section>

    <details className="group rounded-2xl border border-slate-200 bg-white">
      <summary className="flex cursor-pointer list-none items-center gap-3 p-6"><Settings2 className="h-5 w-5 text-slate-400" /><div className="min-w-0 flex-1"><h2 className="text-sm font-semibold">Réglages avancés</h2><p className="mt-1 text-xs text-slate-500">Canal, automatisation et diagnostic de la passerelle</p></div><ChevronDown className="h-5 w-5 text-slate-400 transition group-open:rotate-180" /></summary>
      <div className="space-y-8 border-t border-slate-100 p-6">
        <section><h3 className="font-semibold">Canal de la recherche manuelle</h3><p className="mt-1 text-sm text-slate-500">Stable est conseillé pour une utilisation quotidienne.</p><select aria-label="Canal de mise à jour manuelle" value={channel} disabled={busy} onChange={event => { setChannel(event.target.value as 'stable' | 'edge'); setConfirmInstall(false); }} className={`${inputClass} max-w-md`}><option value="stable">Stable — versions publiées</option><option value="edge">Développement — dernières nouveautés (Edge)</option></select></section>
        {settings && <section className="border-t border-slate-100 pt-7"><div className="flex items-center gap-2"><Clock3 className="h-4 w-4 text-[#007aff]" /><h3 className="font-semibold">Préférences de mise à jour</h3></div><p className="mt-2 text-sm text-slate-500">Le mode « Prévenir seulement » recherche les nouveautés sans les installer. Vos préférences existantes restent appliquées.</p>
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <label className="text-sm font-medium">Installation<select value={settings.mode} onChange={event => { setSettings({ ...settings, mode: event.target.value as UpdateSettings['mode'] }); setSettingsSaved(false); }} className={inputClass}><option value="notify">Prévenir seulement — installation manuelle</option><option value="off">Ne pas rechercher automatiquement</option><option value="automatic">Installer automatiquement</option></select></label>
            <label className="text-sm font-medium">Canal surveillé<select value={settings.channel} onChange={event => { setSettings({ ...settings, channel: event.target.value as UpdateSettings['channel'], allow_edge_automatic: event.target.value === 'edge' ? settings.allow_edge_automatic : false }); setSettingsSaved(false); }} className={inputClass}><option value="stable">Stable</option><option value="edge">Développement (Edge)</option></select></label>
            <label className="text-sm font-medium">À partir de<input type="time" value={settings.window_start} onChange={event => { setSettings({ ...settings, window_start: event.target.value }); setSettingsSaved(false); }} className={inputClass} /></label>
            <label className="text-sm font-medium">Jusqu’à<input type="time" value={settings.window_end} onChange={event => { setSettings({ ...settings, window_end: event.target.value }); setSettingsSaved(false); }} className={inputClass} /></label>
            <label className="text-sm font-medium">Délai après publication (heures)<input type="number" min="1" max="720" value={settings.delay_hours} onChange={event => { setSettings({ ...settings, delay_hours: Number(event.target.value) }); setSettingsSaved(false); }} className={inputClass} /></label>
            <div><p className="text-sm font-medium">Jours autorisés</p><div className="mt-2 flex flex-wrap gap-1.5">{[{ label: 'Lun', value: 1 }, { label: 'Mar', value: 2 }, { label: 'Mer', value: 3 }, { label: 'Jeu', value: 4 }, { label: 'Ven', value: 5 }, { label: 'Sam', value: 6 }, { label: 'Dim', value: 0 }].map(day => <button key={day.value} aria-pressed={settings.allowed_days.includes(day.value)} onClick={() => { setSettings({ ...settings, allowed_days: settings.allowed_days.includes(day.value) ? settings.allowed_days.filter(value => value !== day.value) : [...settings.allowed_days, day.value].sort() }); setSettingsSaved(false); }} className={`min-h-11 rounded-xl px-3 text-xs font-medium ${settings.allowed_days.includes(day.value) ? 'bg-[#007aff] text-white' : 'bg-[#f5f5f7] text-slate-500'}`}>{day.label}</button>)}</div></div>
          </div>
          <div className="mt-5 grid gap-2 lg:grid-cols-3"><PolicyCheck checked={settings.prevent_robot_active} label="Attendre que les robots soient inactifs" onChange={value => { setSettings({ ...settings, prevent_robot_active: value }); setSettingsSaved(false); }} /><PolicyCheck checked={settings.prevent_cleaning} label="Attendre la fin des nettoyages" onChange={value => { setSettings({ ...settings, prevent_cleaning: value }); setSettingsSaved(false); }} /><PolicyCheck checked={settings.prevent_command_in_progress} label="Attendre la fin des commandes" onChange={value => { setSettings({ ...settings, prevent_command_in_progress: value }); setSettingsSaved(false); }} /></div>
          {settings.mode === 'automatic' && settings.channel === 'edge' && <label className="mt-4 flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm"><input type="checkbox" checked={settings.allow_edge_automatic} onChange={event => { setSettings({ ...settings, allow_edge_automatic: event.target.checked }); setSettingsSaved(false); }} className="mt-1 accent-[#007aff]" /><span>J’autorise explicitement l’installation automatique des versions de développement (Edge).</span></label>}
          <div className="mt-5 flex flex-wrap items-center gap-4"><button onClick={() => void savePolicy()} disabled={busy || settings.allowed_days.length === 0 || (settings.mode === 'automatic' && settings.channel === 'edge' && !settings.allow_edge_automatic)} className={primaryButton}>{settingsSaved ? <Check className="h-4 w-4" /> : null}{action === 'settings' ? 'Enregistrement…' : settingsSaved ? 'Préférences enregistrées' : 'Enregistrer les préférences'}</button>{settings.allowed_days.length === 0 && <p className="text-xs text-amber-700">Choisissez au moins un jour.</p>}</div>
        </section>}
        <section className="border-t border-slate-100 pt-7"><div className="flex items-center gap-2"><Activity className="h-4 w-4 text-[#007aff]" /><h3 className="font-semibold">Diagnostic technique</h3></div>
          <div className="mt-4 grid gap-6 lg:grid-cols-2"><div><KeyValue label="Version / canal" value={`${status.version} / ${status.channel}`} /><KeyValue label="Révision" value={shortCommit(status.git_commit)} /><KeyValue label="Date de compilation" value={formatDate(status.build_time)} /><KeyValue label="Architecture" value={`${status.architecture} · ${status.go_version}`} /><KeyValue label="Dernier redémarrage" value={formatDate(status.last_restart)} /><KeyValue label="Récupération automatique" value={status.last_watchdog_reason || 'Aucune enregistrée'} /><KeyValue label="Superviseur" value={`${status.supervisor.kind} · ${status.supervisor.log_mode}`} /></div><div>{Object.entries(status.transports).map(([name, transport]) => <div key={name} className="border-b border-slate-100 py-3 last:border-0"><div className="flex justify-between gap-3"><span className="text-sm font-medium">{transportName(name)}</span><span className={`text-xs ${!transport.enabled ? 'text-slate-400' : transport.connected ? 'text-[#007aff]' : 'text-amber-700'}`}>{!transport.enabled ? 'Désactivé' : transport.connected ? 'Connecté' : 'À vérifier'}</span></div>{transport.last_success && <p className="mt-1 text-xs text-slate-500">Dernier échange : {formatDate(transport.last_success)}</p>}{transport.last_error && <p className="mt-2 break-words text-xs text-amber-700">{transport.last_error}</p>}</div>)}</div></div>
          {failedDirectInputs.length > 0 && <div className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-4"><p className="text-sm font-medium">Entrées refusées par le Miniserver</p><p className="mt-1 text-xs leading-relaxed text-amber-800">Vérifiez leur nom exact dans Loxone Config, puis enregistrez le projet dans le Miniserver.</p><div className="mt-3 space-y-2">{failedDirectInputs.map(input => <p key={`${input.name}-${input.field}`} className="break-all text-xs"><code>{input.name}</code> — {input.error}</p>)}</div><Link to="/loxone" className="mt-4 inline-flex items-center gap-2 text-sm font-medium text-[#007aff]">Ouvrir la connexion Loxone<ArrowRight className="h-4 w-4" /></Link></div>}
          <div className="mt-5 grid gap-2 sm:grid-cols-2">{Object.entries(status.health.components || {}).map(([name, component]) => <div key={name} className="rounded-xl bg-[#f5f5f7] p-3"><div className="flex justify-between gap-2"><span className="text-xs font-medium">{componentName(name)}</span><span className={`text-xs ${component.healthy ? 'text-[#007aff]' : component.required ? 'text-amber-700' : 'text-slate-400'}`}>{component.healthy ? 'OK' : component.required ? 'À vérifier' : 'Optionnel'}</span></div>{component.detail && <p className="mt-1 break-words text-xs text-slate-500">{component.detail}</p>}</div>)}</div>
          <div className="mt-5 rounded-xl bg-[#f5f5f7] p-4 text-xs leading-relaxed text-slate-500"><p>Planificateur : {status.auto_update.last_decision || 'En attente de la première vérification.'}</p>{status.auto_update.next_window && <p className="mt-1">Prochaine plage : {formatDate(status.auto_update.next_window)}</p>}{status.auto_update.last_error && <p className="mt-1 text-amber-700">{status.auto_update.last_error}</p>}<p className="mt-2">Les journaux complets sont disponibles dans Container Manager → roborock-mqtt-loxone → Journal.</p></div>
        </section>
      </div>
    </details>
  </div>;
}

function ServiceCard({ icon, label, value, detail, good }: { icon: ReactNode; label: string; value: string; detail: string; good: boolean }) {
  return <div className="rounded-2xl border border-slate-200 bg-white p-5"><div className="flex items-center gap-3"><span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${good ? 'bg-[#f0f6ff] text-[#007aff]' : 'bg-amber-50 text-amber-600'}`}>{icon}</span><div><p className="text-xs text-slate-500">{label}</p><p className="mt-1 text-sm font-semibold">{value}</p></div></div><p className="mt-3 text-xs leading-relaxed text-slate-500">{detail}</p></div>;
}
function KeyValue({ label, value }: { label: string; value: string }) {
  return <div className="flex flex-wrap items-start justify-between gap-2 border-b border-slate-100 py-3 text-xs last:border-0"><span className="text-slate-500">{label}</span><span className="break-all text-right">{value}</span></div>;
}
function PolicyCheck({ checked, label, onChange }: { checked: boolean; label: string; onChange: (value: boolean) => void }) {
  return <label className="flex items-start gap-2.5 rounded-xl border border-slate-200 p-3 text-xs leading-relaxed"><input type="checkbox" checked={checked} onChange={event => onChange(event.target.checked)} className="mt-0.5 accent-[#007aff]" /><span>{label}</span></label>;
}
function UpdateProgress({ operation, reconnecting }: { operation: UpdateOperation; reconnecting: boolean }) {
  const stages = ['preparing', 'pulling', 'backing_up', 'restarting', 'validating', 'success'];
  const labels: Record<string, string> = { preparing: 'Préparation', pulling: 'Téléchargement', backing_up: 'Sauvegarde', restarting: 'Redémarrage', validating: 'Vérification', success: 'Terminé', rollback: 'Restauration', failed: 'Échec' };
  const current = stages.indexOf(operation.stage);
  const failed = operation.stage === 'failed';
  const complete = operation.stage === 'success';
  return <section role="status" aria-live="polite" className={`rounded-2xl border bg-white p-6 ${failed ? 'border-red-200' : 'border-slate-200'}`}><div className="flex items-center gap-3"><span className={`flex h-10 w-10 items-center justify-center rounded-xl ${failed ? 'bg-red-50 text-red-600' : 'bg-[#f0f6ff] text-[#007aff]'}`}>{complete ? <CheckCircle2 className="h-5 w-5" /> : failed ? <AlertCircle className="h-5 w-5" /> : <RefreshCw className="h-5 w-5 animate-spin" />}</span><div><h2 className="font-semibold">{failed ? 'La mise à jour n’a pas abouti' : operation.stage === 'rollback' ? 'Restauration de la version précédente' : complete ? 'Votre passerelle a été mise à jour' : 'Installation en cours'}</h2><p className="mt-1 text-xs text-slate-500">{reconnecting ? 'La passerelle redémarre. Reconnexion automatique en cours…' : complete ? 'Les échanges avec Loxone peuvent reprendre.' : 'Vous pouvez revenir sur cette page pour retrouver la progression.'}</p></div></div><div className="mt-5 grid grid-cols-3 gap-2 sm:grid-cols-6">{stages.map((stage, index) => <div key={stage} className={`rounded-lg px-2 py-2.5 text-center text-[11px] ${index <= current ? 'bg-[#f0f6ff] font-medium text-[#007aff]' : 'bg-[#f5f5f7] text-slate-400'}`}>{labels[stage]}</div>)}</div>{operation.error && <p className="mt-4 break-words text-sm text-red-700">{operation.error}</p>}{operation.rollback_error && <p className="mt-2 break-words text-sm text-red-700">Restauration : {operation.rollback_error}</p>}<p className="mt-4 text-xs text-slate-400">{labels[operation.stage] || operation.stage} · {formatDate(operation.updated_at)}</p></section>;
}

const errorMessage = (reason: unknown, fallback: string) => reason instanceof Error ? reason.message : fallback;
const shortCommit = (value: string) => value.length > 12 ? value.slice(0, 12) : value;
const formatDate = (value?: string) => !value || value === 'unknown' || Number.isNaN(Date.parse(value)) ? 'Non disponible' : new Date(value).toLocaleString('fr-FR');
const formatDuration = (seconds: number) => { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return days > 0 ? `${days} j ${hours} h` : hours > 0 ? `${hours} h ${minutes} min` : `${minutes} min`; };
const formatBytes = (bytes: number) => bytes > 1024 ** 3 ? `${(bytes / 1024 ** 3).toLocaleString('fr-FR', { maximumFractionDigits: 1 })} Go` : `${(bytes / 1024 ** 2).toFixed(0)} Mo`;
const transportName = (name: string) => ({ direct: 'Loxone HTTP', loxone: 'Loxone MQTT', mqtt: 'MQTT', cloud: 'Roborock Cloud', roborock: 'Roborock Cloud' }[name] || name);
const componentName = (name: string) => ({ cloud: 'Roborock Cloud', roborock: 'Roborock', mqtt: 'MQTT', direct: 'Loxone HTTP', loxone_direct: 'Loxone HTTP', loxone: 'Loxone MQTT', storage: 'Stockage', data_volume: 'Données', polling: 'Actualisation des robots', web: 'Interface web' }[name] || name.replace(/_/g, ' '));
