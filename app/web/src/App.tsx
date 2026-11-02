import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, NavLink, Navigate, Route, Routes, useParams } from 'react-router-dom';
import { ArrowDownToLine, ArrowRight, ArrowUpRight, Battery, Bot, Check, ChevronRight, CircleHelp, House, LayoutGrid, LoaderCircle, Menu, Network, Pause, Play, Plus, RefreshCw, Settings2, ShieldCheck, X } from 'lucide-react';
import { useSSE } from '@/hooks/useSSE';
import { fetchDevices, fetchLoxoneIntegration, fetchScenes, fetchSetupStatus, getAuthStatus, testLoxoneCommand } from '@/lib/api';
import type { SceneInfo } from '@/lib/api';
import type { DeviceSummary } from '@/types/status';
import type { LoxoneActivity, LoxoneIntegration } from '@/types/loxone';
import { LoginPage } from '@/components/LoginPage';
import { SetupWizard } from '@/components/SetupWizard';
import { LoxoneIntegrationPage } from '@/components/LoxoneIntegrationPage';
import { SystemUpdatesPage } from '@/components/SystemUpdatesPage';
import { ConfirmModal } from '@/components/ConfirmModal';
import { SchedulePage } from '@/components/SchedulePage';
import { robotStateLabel, robotNeedsAttention } from '@/lib/presentation';

export function App() {
  const [ready, setReady] = useState(false);
  const [authenticated, setAuthenticated] = useState(false);
  const [setupComplete, setSetupComplete] = useState(false);
  const [initialError, setInitialError] = useState('');
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const [integration, setIntegration] = useState<LoxoneIntegration | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState('');
  const [menuOpen, setMenuOpen] = useState(false);
  const live = useSSE();

  const initialize = useCallback(async () => {
    setInitialError('');
    try {
      const [setup, auth] = await Promise.all([fetchSetupStatus(), getAuthStatus()]);
      setSetupComplete(setup.setup_complete);
      setAuthenticated(auth.authenticated);
      setReady(true);
    } catch {
      setInitialError('Le bridge ne répond pas pour le moment. Vos réglages sont conservés.');
    }
  }, []);
  useEffect(() => { void initialize(); }, [initialize]);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const [nextDevices, nextIntegration] = await Promise.all([fetchDevices(), fetchLoxoneIntegration()]);
      setDevices(nextDevices);
      setIntegration(nextIntegration);
      setRefreshError('');
    } catch {
      setRefreshError('Impossible d’actualiser les robots. Réessayez dans un instant.');
    } finally { setRefreshing(false); }
  }, []);
  useEffect(() => {
    if (!authenticated || !setupComplete) return;
    void refresh();
    const timer = window.setInterval(() => void refresh(), 30000);
    return () => window.clearInterval(timer);
  }, [authenticated, setupComplete, refresh]);

  const robots = useMemo(() => devices.map(device => ({ ...device, online: live.availabilities[device.slug] ?? device.online, status: live.statuses[device.slug] ?? device.status })), [devices, live.availabilities, live.statuses]);
  if (!ready) return <div className="flex min-h-screen items-center justify-center p-6"><div className="max-w-md text-center"><BridgeMark /><p className="mt-5 text-sm text-muted-foreground">{initialError || 'Connexion à votre passerelle…'}</p>{initialError ? <button className="bridge-button-secondary mt-4" onClick={() => void initialize()}>Réessayer</button> : <LoaderCircle className="mx-auto mt-4 h-5 w-5 animate-spin text-primary" />}</div></div>;
  if (!setupComplete) return <SetupWizard onComplete={() => void initialize()} />;
  if (!authenticated) return <LoginPage onLogin={() => { setAuthenticated(true); void refresh(); }} />;

  const navigation = [{ to: '/', label: 'Mes robots', icon: LayoutGrid }, { to: '/loxone', label: 'Loxone', icon: Network }, { to: '/system', label: 'Mises à jour', icon: ArrowDownToLine }];
  return <div className="bridge-app min-h-screen">
    <aside className={`bridge-sidebar ${menuOpen ? 'is-open' : ''}`}>
      <Link to="/" onClick={() => setMenuOpen(false)} className="flex items-center gap-3 px-2"><BridgeMark /><div><strong className="block text-[17px] tracking-tight">Roborock <span className="font-normal text-muted-foreground">Bridge</span></strong><span className="text-[11px] tracking-[.12em] text-muted-foreground">POUR LOXONE</span></div></Link>
      <div className="mt-12 px-3 text-[10px] font-semibold uppercase tracking-[.16em] text-muted-foreground">Votre installation</div>
      <nav className="mt-3 space-y-1.5" aria-label="Navigation principale">{navigation.map(({ to, label, icon: Icon }) => <NavLink key={to} to={to} end={to === '/'} onClick={() => setMenuOpen(false)} className={({ isActive }) => `bridge-nav ${isActive ? 'is-active' : ''}`}><Icon className="h-[19px] w-[19px]" />{label}<ChevronRight className="ml-auto h-4 w-4 opacity-40" /></NavLink>)}</nav>
      <div className="mt-8 rounded-2xl bg-[#f0f5f2] p-4"><div className="flex items-center gap-2 text-xs font-semibold"><span className={`h-2 w-2 rounded-full ${live.isConnected ? 'bg-emerald-500' : 'bg-amber-500'}`} />{live.isConnected ? 'Bridge connecté' : 'Reconnexion en cours'}</div><p className="mt-2 text-xs leading-relaxed text-muted-foreground">{robots.length} robot{robots.length > 1 ? 's' : ''} · {integration?.direct_enabled ? 'Connexion directe Loxone' : integration?.enabled ? 'Connexion MQTT Loxone' : 'Connexion à configurer'}</p></div>
      <div className="mt-auto space-y-1 pt-12"><NavLink to="/setup" onClick={() => setMenuOpen(false)} className={({ isActive }) => `bridge-nav ${isActive ? 'is-active' : ''}`}><Settings2 className="h-[18px] w-[18px]" />Réglages</NavLink><Link to="/loxone" onClick={() => setMenuOpen(false)} className="bridge-nav"><CircleHelp className="h-[18px] w-[18px]" />Guide de connexion</Link><div className="px-3 pt-5 text-[10px] leading-5 text-muted-foreground">Roborock → Loxone<br />Basé sur mqtt-home/roborock-mqtt</div></div>
    </aside>
    {menuOpen && <button className="fixed inset-0 z-30 bg-black/25 lg:hidden" aria-label="Fermer le menu" onClick={() => setMenuOpen(false)} />}
    <div className="bridge-main">
      <header className="bridge-topbar"><div className="flex items-center gap-3"><button className="bridge-icon-button lg:hidden" aria-label={menuOpen ? 'Fermer le menu' : 'Ouvrir le menu'} onClick={() => setMenuOpen(value => !value)}>{menuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}</button><span className="text-xs text-muted-foreground">Espace maison <span className="mx-2 opacity-40">/</span><span className="font-medium text-foreground">Passerelle Loxone</span></span></div><span className="inline-flex items-center gap-2 text-xs text-muted-foreground"><span className={`h-1.5 w-1.5 rounded-full ${live.isConnected ? 'bg-emerald-500' : 'bg-amber-500'}`} />{live.isConnected ? 'En direct' : 'Reconnexion'}</span></header>
      <main id="main-content" className="bridge-content">
        {(refreshError || (!live.isConnected && live.error)) && <div role="status" className="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"><span>{refreshError || 'La connexion en direct est interrompue. Les états affichés peuvent être anciens.'}</span><button className="font-semibold underline" onClick={() => { live.reconnect(); void refresh(); }}>Reconnecter</button></div>}
        <Routes>
          <Route path="/" element={<FleetHome robots={robots} integration={integration} refreshing={refreshing} refresh={refresh} connected={live.isConnected} />} />
          <Route path="/devices/:slug/*" element={<RobotWorkspace robots={robots} integration={integration} activities={live.loxoneActivities} refresh={refresh} />} />
          <Route path="/loxone" element={<LoxoneIntegrationPage liveActivities={live.loxoneActivities} liveStatuses={live.statuses} liveAvailabilities={live.availabilities} streamConnected={live.isConnected} />} />
          <Route path="/system" element={<SystemUpdatesPage />} />
          <Route path="/setup" element={<SetupWizard reconfigure onComplete={() => { void initialize(); void refresh(); }} />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  </div>;
}

function BridgeMark() { return <span className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-[14px] bg-[#173d2b] text-white shadow-sm"><Network className="h-[23px] w-[23px]" strokeWidth={1.6} /></span>; }

function FleetHome({ robots, integration, refreshing, refresh, connected }: { robots: DeviceSummary[]; integration: LoxoneIntegration | null; refreshing: boolean; refresh: () => Promise<void>; connected: boolean }) {
  const errors = robots.filter(robot => robotNeedsAttention(robot.status));
  const directSuccess = integration?.direct_diagnostics?.last_transmission;
  const loxoneReady = Boolean(integration?.direct_enabled ? directSuccess && Date.parse(directSuccess) > 0 && !integration.direct_diagnostics?.last_error : integration?.enabled && integration.mqtt_test?.ok);
  return <div className="space-y-8">
    <div className="flex flex-wrap items-end justify-between gap-4"><div><p className="bridge-eyebrow">Votre passerelle, tout simplement</p><h1 className="bridge-title">Mes robots<span className="text-primary">.</span></h1><p className="bridge-description">Vos Roborock, réunis et prêts pour Loxone.</p></div><div className="flex gap-2"><button className="bridge-icon-button" onClick={() => void refresh()} aria-label="Actualiser les robots" disabled={refreshing}><RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} /></button><Link className="bridge-button-primary" to="/setup?section=robots"><Plus className="h-4 w-4" />Ajouter un robot</Link></div></div>
    <section className="bridge-hero"><div className="relative z-10"><span className="inline-flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-3 py-1.5 text-[11px] font-medium text-emerald-100"><span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-300' : 'bg-amber-300'}`} />{connected ? 'Votre passerelle est en ligne' : 'Connexion en cours'}</span><h2 className="mt-5 max-w-sm text-[28px] font-semibold leading-[1.2] tracking-tight sm:text-[34px]">La maison s’occupe<br />du reste.</h2><p className="mt-3 max-w-sm text-sm leading-6 text-emerald-100/75">Gardez vos automatismes dans Loxone.<br />Le bridge fait le lien avec vos robots.</p><Link to="/loxone" className="mt-6 inline-flex items-center gap-2 text-sm font-medium text-white">Voir ma connexion Loxone <ArrowRight className="h-4 w-4" /></Link></div><div className="bridge-hero-art" aria-hidden="true"><div className="bridge-orbit orbit-one" /><div className="bridge-orbit orbit-two" /><div className="bridge-orbit orbit-three" /><div className="hero-robot"><RobotDrawing /><span className="hero-mini-badge"><Check className="h-4 w-4" /></span></div><span className="hero-loxone">L<span className="hero-loxone-dot" /></span><span className="hero-flow-dot" /></div></section>
    <div className="grid gap-3 sm:grid-cols-3"><Metric icon={<Bot className="h-4 w-4" />} title="Robots connectés" value={`${robots.filter(robot => robot.online).length} / ${robots.length}`} note="Sur votre compte Roborock" /><Metric icon={<Network className="h-4 w-4" />} title="Connexion Loxone" value={loxoneReady ? 'Opérationnelle' : integration?.direct_enabled || integration?.enabled ? 'À vérifier' : 'À configurer'} note={integration?.direct_enabled ? 'HTTP direct · sans intermédiaire' : integration?.enabled ? 'Via votre serveur MQTT' : 'Ajoutez votre Miniserver'} good={loxoneReady} /><Metric icon={<ShieldCheck className="h-4 w-4" />} title="À surveiller" value={errors.length ? `${errors.length} robot${errors.length > 1 ? 's' : ''}` : 'Tout est en ordre'} note={errors.length ? 'Consultez les détails ci-dessous' : 'Aucune erreur robot signalée'} good={!errors.length} /></div>
    <section><div className="mb-4 flex items-center justify-between"><h2 className="text-lg font-semibold tracking-tight">Vos robots <span className="ml-1 text-sm font-normal text-muted-foreground">{robots.length.toString().padStart(2, '0')}</span></h2><span className="text-xs text-muted-foreground">Mise à jour en direct</span></div>{robots.length ? <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">{robots.map(robot => <RobotCard key={robot.slug} robot={robot} integration={integration} />)}<Link to="/setup?section=robots" className="group flex min-h-[290px] flex-col items-center justify-center rounded-[22px] border border-dashed border-[#cfdad2] bg-white/30 p-7 text-center transition hover:border-primary hover:bg-white"><span className="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-white text-primary transition group-hover:bg-primary group-hover:text-white"><Plus className="h-5 w-5" /></span><strong className="mt-4 text-sm font-medium">Un autre Roborock ?</strong><span className="mt-2 max-w-[190px] text-xs leading-5 text-muted-foreground">Connectez les robots du même compte, avec leurs propres adresses Loxone.</span></Link></div> : <div className="bridge-card p-10 text-center"><Bot className="mx-auto h-10 w-10 text-primary" /><h3 className="mt-4 font-semibold">Votre premier robot vous attend</h3><p className="mt-2 text-sm text-muted-foreground">Ajoutez votre robot dans Roborock avec le même compte, puis redémarrez le conteneur du bridge sur votre Synology.</p><button className="bridge-button-primary mt-5" onClick={() => void refresh()}>Rechercher mes robots</button></div>}</section>
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-white/60 px-5 py-4"><div className="flex items-center gap-3"><ShieldCheck className="h-5 w-5 text-primary" /><p className="text-xs text-muted-foreground">Vos adresses et entrées Loxone restent les mêmes, y compris après une mise à jour.</p></div><Link to="/system" className="inline-flex items-center gap-1 text-xs font-semibold text-primary">Mises à jour <ArrowUpRight className="h-3.5 w-3.5" /></Link></div>
  </div>;
}

function Metric({ icon, title, value, note, good }: { icon: React.ReactNode; title: string; value: string; note: string; good?: boolean }) { return <div className="bridge-card px-5 py-4"><div className="flex items-center gap-2 text-xs text-muted-foreground">{icon}{title}</div><p className={`mt-3 text-lg font-semibold tracking-tight ${good ? 'text-[#247449]' : ''}`}>{value}</p><p className="mt-1 text-[11px] text-muted-foreground">{note}</p></div>; }

function RobotCard({ robot, integration }: { robot: DeviceSummary; integration: LoxoneIntegration | null }) {
  const connection = integration?.robots.find(item => item.slug === robot.slug);
  const needsAttention = robotNeedsAttention(robot.status);
  const dockError = robot.status?.dock_error_status;
  return <Link to={`/devices/${robot.slug}`} className="bridge-card group block overflow-hidden p-5 transition hover:-translate-y-1 hover:border-primary/30 hover:shadow-lg hover:shadow-emerald-950/5"><div className="flex items-center justify-between"><span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-medium ${!robot.online ? 'bg-slate-100 text-slate-500' : needsAttention ? 'bg-amber-50 text-amber-700' : 'bg-emerald-50 text-emerald-700'}`}><span className="h-1.5 w-1.5 rounded-full bg-current" />{!robot.online ? 'Hors ligne' : needsAttention ? 'À vérifier' : robotStateLabel(robot.status?.state)}</span><ArrowUpRight className="h-4 w-4 text-muted-foreground transition group-hover:text-primary" /></div><div className="flex h-[142px] items-center justify-center"><div className="w-[112px]"><RobotDrawing /></div></div><h3 className="truncate text-base font-semibold tracking-tight">{robot.name}</h3><p className="mt-1 truncate text-[11px] text-muted-foreground">{robot.model}</p>{needsAttention && <p className="mt-2 text-xs text-amber-700">{robot.status?.error || (dockError ? `La station signale une erreur (${dockError}).` : `Erreur robot ${robot.status?.error_code}`)}</p>}<div className="mt-4 flex items-center justify-between border-t border-border pt-4 text-xs"><span className="inline-flex items-center gap-1.5 text-muted-foreground"><Battery className="h-4 w-4" />{robot.status ? `${robot.status.battery} %` : '—'}</span><span className={`inline-flex items-center gap-1.5 ${connection?.direct_enabled || connection?.mqtt_enabled ? 'text-primary' : 'text-muted-foreground'}`}><Network className="h-3.5 w-3.5" />{connection?.direct_enabled || connection?.mqtt_enabled ? 'Relié à Loxone' : 'Loxone à configurer'}</span></div></Link>;
}

function RobotWorkspace({ robots, integration, activities, refresh }: { robots: DeviceSummary[]; integration: LoxoneIntegration | null; activities: Record<string, LoxoneActivity>; refresh: () => Promise<void> }) {
  const { slug } = useParams<{ slug: string }>();
  const robot = robots.find(item => item.slug === slug);
  const [scenes, setScenes] = useState<SceneInfo[]>([]);
  const [busy, setBusy] = useState('');
  const [notice, setNotice] = useState<{ error: boolean; text: string } | null>(null);
  const [pending, setPending] = useState<{ command: string; name: string } | null>(null);
  useEffect(() => { setScenes([]); if (slug) fetchScenes(slug).then(setScenes).catch(() => {}); }, [slug]);
  const run = async (command: string) => {
    if (!slug) return;
    setBusy(command); setNotice(null);
    try { await testLoxoneCommand(slug, command); setNotice({ error: false, text: 'Commande transmise au robot. Son état se mettra à jour après confirmation.' }); void refresh(); }
    catch (error) { setNotice({ error: true, text: error instanceof Error ? error.message : 'La commande n’a pas pu être envoyée.' }); }
    finally { setBusy(''); }
  };
  if (!robot) return <div className="bridge-card p-10 text-center"><Bot className="mx-auto h-8 w-8 text-primary" /><h1 className="mt-4 text-xl font-semibold">Robot indisponible</h1><p className="mt-2 text-sm text-muted-foreground">Actualisez la liste ou vérifiez votre connexion Roborock.</p><Link className="bridge-button-secondary mt-5" to="/">Mes robots</Link></div>;
  const current = integration?.robots.find(item => item.slug === slug);
  const latest = activities[robot.slug] ?? current?.diagnostics.last_activity;
  return <div className="space-y-6"><Link to="/" className="inline-flex items-center gap-2 text-xs text-muted-foreground">Mes robots <ChevronRight className="h-3 w-3" />{robot.name}</Link><div className="flex flex-wrap items-center justify-between gap-3"><div><p className="bridge-eyebrow">Votre robot</p><h1 className="bridge-title">{robot.name}</h1><p className="bridge-description">{robot.model}</p></div><Link to={`/loxone?robot=${encodeURIComponent(robot.slug)}`} className="bridge-button-secondary"><Network className="h-4 w-4" />Connexion Loxone <ArrowUpRight className="h-4 w-4" /></Link></div><div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.25fr)]"><section className="bridge-card p-7"><div className="mx-auto my-4 w-[180px]"><RobotDrawing /></div><div className="mt-6 flex items-center justify-between"><span className="rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-medium text-primary">{robot.online ? robotStateLabel(robot.status?.state) : 'Hors ligne'}</span><span className="inline-flex items-center gap-2 text-sm"><Battery className="h-4 w-4 text-muted-foreground" />{robot.status ? `${robot.status.battery} %` : '—'}</span></div>{robotNeedsAttention(robot.status) && <div role="status" className="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">{robot.status?.error || `Erreur signalée. Consultez l’application Roborock pour le détail.`}</div>}<div className="mt-6 grid grid-cols-3 gap-2">{[{ command: 'start', name: 'Démarrer', icon: Play }, { command: 'pause', name: 'Pause', icon: Pause }, { command: 'dock', name: 'Station', icon: House }].map(({ command, name, icon: Icon }) => <button key={command} disabled={Boolean(busy) || !robot.online} onClick={() => command === 'start' ? setPending({ command, name }) : void run(command)} className={command === 'start' ? 'bridge-button-primary flex-col gap-2 py-4' : 'bridge-button-secondary flex-col gap-2 py-4'}>{busy === command ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Icon className="h-4 w-4" />}<span className="text-xs">{name}</span></button>)}</div>{notice && <p role={notice.error ? 'alert' : 'status'} className={`mt-4 text-xs leading-5 ${notice.error ? 'text-red-600' : 'text-primary'}`}>{notice.text}</p>}</section><section className="bridge-card p-6"><div className="flex items-center justify-between"><h2 className="text-lg font-semibold">Programmes Roborock</h2><span className="text-xs text-muted-foreground">{scenes.length} disponible{scenes.length > 1 ? 's' : ''}</span></div><p className="mt-2 text-xs leading-5 text-muted-foreground">Vos routines créées dans l’application Roborock, disponibles aussi depuis Loxone.</p><div className="mt-5 space-y-2">{scenes.length ? scenes.map(scene => <button key={scene.id} disabled={Boolean(busy) || !robot.online} onClick={() => setPending({ command: `scene_id:${scene.id}`, name: scene.name })} className="group flex w-full items-center gap-3 rounded-xl border border-border px-4 py-3.5 text-left transition hover:border-primary/40 hover:bg-emerald-50/40 disabled:opacity-50"><span className="flex h-9 w-9 items-center justify-center rounded-lg bg-[#f0f5f2] text-primary"><Play className="h-3.5 w-3.5" /></span><span className="flex-1 text-sm font-medium">{scene.name}</span><ArrowUpRight className="h-4 w-4 text-muted-foreground" /></button>) : <div className="rounded-xl border border-dashed border-border p-6 text-center text-sm text-muted-foreground">Créez une routine dans Roborock pour la retrouver ici.</div>}</div>{latest && <div className="mt-6 border-t border-border pt-4"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Dernier retour du robot</p><p className="mt-2 text-xs">{latest.type === 'command' ? `${latest.command} · ${latest.state}` : robotStateLabel(latest.event)}{latest.error_text && ` · ${latest.error_text}`}</p></div>}</section></div><details className="bridge-card p-5"><summary className="cursor-pointer text-sm font-medium">Informations techniques et anciens plannings</summary><div className="mt-4 space-y-3 text-xs text-muted-foreground"><p>Identifiant permanent : <code className="break-all">{robot.slug}</code></p><p>Les anciens plannings du bridge restent conservés. Gérez les nouveaux automatismes dans Loxone.</p><Routes><Route path="schedule" element={<SchedulePage slug={robot.slug} deviceName={robot.name} scenes={scenes} onClose={() => window.history.back()} />} /><Route path="*" element={<Link className="inline-flex items-center gap-1 font-medium text-primary" to={`/devices/${robot.slug}/schedule`}>Consulter les anciens plannings <ArrowRight className="h-3 w-3" /></Link>} /></Routes></div></details><ConfirmModal open={pending !== null} title={`Lancer « ${pending?.name ?? ''} » ?`} message={`Le programme sera envoyé à ${robot.name}.`} confirmLabel="Lancer le programme" onCancel={() => setPending(null)} onConfirm={() => { if (pending) void run(pending.command); setPending(null); }} /></div>;
}

function RobotDrawing() { return <svg viewBox="0 0 200 200" className="w-full drop-shadow-[0_14px_12px_rgba(24,49,36,0.12)]" aria-hidden="true"><defs><linearGradient id="robot-body" x1="0" y1="0" x2="1" y2="1"><stop stopColor="#fff" /><stop offset="1" stopColor="#dce3de" /></linearGradient></defs><circle cx="100" cy="101" r="83" fill="#d1d9d3" /><circle cx="100" cy="97" r="82" fill="url(#robot-body)" stroke="#c9d2cc" /><circle cx="100" cy="97" r="74" fill="none" stroke="#e1e7e3" /><path d="M43 145h114" stroke="#c5cec7" /><circle cx="100" cy="72" r="27" fill="#e4e9e5" stroke="#c8d1ca" /><circle cx="100" cy="70" r="23" fill="#f8faf8" stroke="#d4ddd6" /><circle cx="100" cy="70" r="14" fill="#d8e0d9" /><circle cx="100" cy="70" r="10" fill="#ecf1ec" /><rect x="87" y="118" width="26" height="10" rx="5" fill="#d5dfd7" /><circle cx="95" cy="123" r="2" fill="#4f8b62" /><circle cx="105" cy="123" r="2" fill="#a9b6ad" /><text x="100" y="154" textAnchor="middle" fill="#89998c" fontFamily="Arial,sans-serif" fontSize="8" letterSpacing="1.5">roborock</text></svg>; }
