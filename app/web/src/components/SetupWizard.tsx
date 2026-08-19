import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { ArrowLeft, Bot, Check, ChevronLeft, ChevronRight, Cloud, KeyRound, LockKeyhole, Network, Radio, Save, Server, ShieldCheck, Sparkles, Wifi } from 'lucide-react';
import { Link } from 'react-router-dom';
import {
  fetchDevices, fetchSetupStatus, loginWithCode, requestCode, rotateDirectToken,
  saveSetupSettings, testDirectSettings, testMQTTSettings,
} from '@/lib/api';
import type { SetupStatus } from '@/lib/api';

const steps = [
  { name: 'Welcome', eyebrow: 'Getting started', description: 'A quick overview before connecting your installation.' },
  { name: 'Roborock', eyebrow: 'Cloud account', description: 'Connect the account used by your Roborock robots.' },
  { name: 'Robots', eyebrow: 'Discovery', description: 'Check the robots detected by the bridge.' },
  { name: 'Mode', eyebrow: 'Integration', description: 'Choose MQTT, Direct HTTP, or both together.' },
  { name: 'MQTT', eyebrow: 'Optional transport', description: 'Connect the local broker used by Loxone.' },
  { name: 'Direct HTTP', eyebrow: 'Miniserver', description: 'Configure state pushes and secured HTTP commands.' },
  { name: 'Review', eyebrow: 'Final check', description: 'Review the complete setup before applying it.' },
  { name: 'Finish', eyebrow: 'Ready', description: 'Persist the configuration and start the integrations.' },
];

type IntegrationMode = 'mqtt' | 'direct' | 'both';

export function SetupWizard({ onComplete, reconfigure = false }: { onComplete: () => void; reconfigure?: boolean }) {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [step, setStep] = useState(reconfigure ? 3 : 0);
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [mqttPassword, setMQTTPassword] = useState('');
  const [directPassword, setDirectPassword] = useState('');
  const [robots, setRobots] = useState<Array<{ slug: string; name: string; model: string }>>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [token, setToken] = useState('');

  useEffect(() => { fetchSetupStatus().then(value => { setStatus(value); setEmail(value.roborock_username); }); }, []);
  useEffect(() => { if (status?.authenticated) fetchDevices().then(setRobots).catch(() => setRobots([])); }, [status?.authenticated]);

  const payload = useMemo(() => status && ({
    setup_complete: false,
    roborock_username: email,
    mqtt: {
      enabled: status.mqtt.enabled, url: status.mqtt.url, retain: status.mqtt.retain,
      topic: status.mqtt.topic, qos: status.mqtt.qos, username: status.mqtt.username,
      password: mqttPassword, tls: status.mqtt.tls,
    },
    loxone: {
      enabled: status.loxone.enabled, topic: status.loxone.topic, devices: status.loxone.devices,
      direct: {
        ...status.loxone.direct, password: directPassword,
        password_configured: undefined, api_token_configured: undefined,
      },
    },
  }), [status, email, mqttPassword, directPassword]);

  if (!status || !payload) return <div className="flex min-h-screen items-center justify-center bg-background text-muted-foreground"><span className="animate-pulse">Loading integration settings…</span></div>;

  const mutate = (fn: (copy: SetupStatus) => void) => {
    const copy = structuredClone(status);
    fn(copy);
    setStatus(copy);
    setMessage(null);
  };
  const save = async (complete = false) => {
    setBusy(true);
    setMessage(null);
    try {
      const saved = await saveSetupSettings({ ...payload, setup_complete: complete });
      setStatus(saved);
      setMQTTPassword('');
      setDirectPassword('');
      setMessage({ ok: true, text: complete ? 'Configuration complete.' : 'Settings saved and applied.' });
      if (complete) onComplete();
    } catch (error) {
      setMessage({ ok: false, text: error instanceof Error ? error.message : 'Save failed' });
    } finally { setBusy(false); }
  };
  const run = async (action: () => Promise<void>, success: string) => {
    setBusy(true);
    setMessage(null);
    try {
      await action();
      setMessage({ ok: true, text: success });
    } catch (error) {
      setMessage({ ok: false, text: error instanceof Error ? error.message : 'Operation failed' });
    } finally { setBusy(false); }
  };
  const mode: IntegrationMode = status.mqtt.enabled && status.loxone.direct.enabled ? 'both' : status.loxone.direct.enabled ? 'direct' : 'mqtt';
  const setMode = (next: IntegrationMode) => mutate(copy => {
    const mqtt = next === 'mqtt' || next === 'both';
    copy.mqtt.enabled = mqtt;
    copy.loxone.enabled = mqtt;
    copy.loxone.direct.enabled = next === 'direct' || next === 'both';
  });
  const current = steps[step];
  const canFinish = status.authenticated && (status.mqtt.enabled || status.loxone.direct.enabled);

  return <div className="min-h-screen bg-background bg-[radial-gradient(circle_at_top_left,rgba(37,99,235,0.10),transparent_32%),radial-gradient(circle_at_top_right,rgba(16,185,129,0.08),transparent_28%)] p-4 md:p-8 lg:p-10">
    <div className="mx-auto max-w-6xl">
      <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-4">
          <div className="rounded-2xl bg-gradient-to-br from-blue-600 to-indigo-600 p-3 text-white shadow-lg shadow-blue-500/20"><Network className="h-7 w-7" /></div>
          <div><div className="flex flex-wrap items-center gap-2"><h1 className="text-2xl font-bold tracking-tight">Integration Setup</h1><Pill tone="blue">roborock-mqtt-loxone</Pill></div><p className="mt-1 text-sm text-muted-foreground">Configure Roborock, MQTT and Direct Loxone from one place.</p></div>
        </div>
        {reconfigure && <Link to="/loxone" className="touch-target inline-flex items-center justify-center gap-2 self-start rounded-xl border border-border bg-card px-4 text-sm font-medium shadow-sm hover:bg-accent sm:self-auto"><ArrowLeft className="h-4 w-4" /> Back to Loxone</Link>}
      </header>

      <div className="mb-6 grid gap-3 sm:grid-cols-3">
        <StatusCard icon={<Bot className="h-5 w-5" />} title="Roborock Cloud" value={status.authenticated ? `${robots.length} robot${robots.length === 1 ? '' : 's'} detected` : 'Not connected'} ok={status.authenticated} />
        <StatusCard icon={<Radio className="h-5 w-5" />} title="MQTT" value={status.mqtt.enabled ? status.mqtt.url || 'Address required' : 'Disabled'} ok={status.mqtt.enabled && Boolean(status.mqtt.url)} muted={!status.mqtt.enabled} />
        <StatusCard icon={<Cloud className="h-5 w-5" />} title="Direct HTTP" value={status.loxone.direct.enabled ? status.loxone.direct.host || 'Address required' : 'Disabled'} ok={status.loxone.direct.enabled && Boolean(status.loxone.direct.host)} muted={!status.loxone.direct.enabled} />
      </div>

      <div className="grid items-start gap-5 lg:grid-cols-[260px_minmax(0,1fr)]">
        <aside className="rounded-2xl border border-border bg-card p-3 shadow-sm lg:sticky lg:top-6">
          <div className="mb-3 px-2 pt-2"><div className="flex items-center justify-between text-xs text-muted-foreground"><span>Setup progress</span><strong className="text-foreground">{step + 1} / {steps.length}</strong></div><div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary transition-all" style={{ width: `${(step + 1) / steps.length * 100}%` }} /></div></div>
          <nav className="grid grid-cols-4 gap-1 lg:block" aria-label="Setup steps">{steps.map((item, index) => <button key={item.name} onClick={() => setStep(index)} className={`group flex min-w-0 items-center gap-2 rounded-xl p-2.5 text-left transition-colors lg:mb-1 lg:w-full lg:gap-3 ${index === step ? 'bg-primary text-primary-foreground shadow-sm' : 'hover:bg-accent'}`} aria-current={index === step ? 'step' : undefined}><span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-xs font-semibold ${index === step ? 'bg-white/20' : index < step ? 'bg-green-500/15 text-green-600' : 'bg-muted text-muted-foreground'}`}>{index < step ? <Check className="h-3.5 w-3.5" /> : index + 1}</span><span className="hidden min-w-0 sm:block"><strong className="block truncate text-xs lg:text-sm">{item.name}</strong><span className={`hidden truncate text-[11px] lg:block ${index === step ? 'text-primary-foreground/70' : 'text-muted-foreground'}`}>{item.eyebrow}</span></span></button>)}</nav>
        </aside>

        <main className="overflow-hidden rounded-2xl border border-border bg-card shadow-xl shadow-slate-950/5">
          <div className="border-b border-border bg-muted/25 px-5 py-5 md:px-7"><p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{current.eyebrow}</p><h2 className="mt-1 text-2xl font-bold">{current.name}</h2><p className="mt-1 text-sm text-muted-foreground">{current.description}</p></div>
          <div className="min-h-[390px] p-5 md:p-7">
            {step === 0 && <WelcomeStep />}
            {step === 1 && <RoborockStep status={status} email={email} code={code} busy={busy} setEmail={setEmail} setCode={setCode} run={run} setStatus={setStatus} />}
            {step === 2 && <RobotsStep robots={robots} authenticated={status.authenticated} />}
            {step === 3 && <ModeStep mode={mode} onChange={setMode} />}
            {step === 4 && <MQTTStep status={status} payload={payload} password={mqttPassword} busy={busy} mutate={mutate} setPassword={setMQTTPassword} run={run} goToMode={() => setStep(3)} />}
            {step === 5 && <DirectStep status={status} payload={payload} password={directPassword} token={token} busy={busy} mutate={mutate} setPassword={setDirectPassword} setToken={setToken} run={run} goToMode={() => setStep(3)} />}
            {step === 6 && <ReviewStep status={status} robots={robots} mode={mode} />}
            {step === 7 && <FinishStep canFinish={canFinish} busy={busy} onFinish={() => void save(true)} />}
            {message && <div role="status" className={`mt-5 rounded-xl border p-3 text-sm ${message.ok ? 'border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300' : 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'}`}>{message.text}</div>}
          </div>
          <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-border bg-muted/25 px-5 py-4 md:px-7">
            <button disabled={step === 0 || busy} className="touch-target inline-flex items-center gap-1 rounded-xl px-3 text-sm font-medium hover:bg-accent disabled:opacity-30" onClick={() => setStep(value => Math.max(0, value - 1))}><ChevronLeft className="h-4 w-4" /> Back</button>
            <div className="flex gap-2">{reconfigure && step !== 7 && <button disabled={busy} onClick={() => void save(false)} className="touch-target inline-flex items-center gap-2 rounded-xl border border-border bg-card px-4 text-sm font-medium hover:bg-accent disabled:opacity-50"><Save className="h-4 w-4" /> Save changes</button>}{step < steps.length - 1 && <button className="touch-target inline-flex items-center gap-1 rounded-xl bg-primary px-4 text-sm font-medium text-primary-foreground shadow-sm hover:opacity-90" onClick={() => setStep(value => Math.min(steps.length - 1, value + 1))}>Next <ChevronRight className="h-4 w-4" /></button>}</div>
          </footer>
        </main>
      </div>
      <p className="mt-5 text-center text-xs text-muted-foreground">Settings are stored in the mounted data volume and applied without rebuilding the container.</p>
    </div>
  </div>;
}

function WelcomeStep() {
  return <div><div className="rounded-2xl border border-blue-500/20 bg-gradient-to-br from-blue-500/10 to-indigo-500/5 p-5"><div className="flex items-start gap-3"><Sparkles className="mt-0.5 h-6 w-6 shrink-0 text-primary" /><div><h3 className="text-lg font-semibold">One bridge, two Loxone transports</h3><p className="mt-1 text-sm leading-relaxed text-muted-foreground">MQTT and Direct HTTP can be enabled independently. Roborock Cloud communication always remains separate from your local Mosquitto broker.</p></div></div></div><div className="mt-5 grid gap-3 md:grid-cols-3"><Feature icon={<Bot />} title="Discover" text="Detect every robot and keep stable slugs." /><Feature icon={<Radio />} title="Connect" text="Use MQTT, Direct HTTP, or both." /><Feature icon={<ShieldCheck />} title="Secure" text="Keep credentials inside the data volume." /></div><div className="mt-5 flex items-start gap-2 rounded-xl border border-amber-500/25 bg-amber-500/10 p-4 text-sm"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" /><span>Back up the data volume: it contains the Roborock session and integration credentials with owner-only file permissions.</span></div></div>;
}

function RoborockStep({ status, email, code, busy, setEmail, setCode, run, setStatus }: { status: SetupStatus; email: string; code: string; busy: boolean; setEmail: (value: string) => void; setCode: (value: string) => void; run: (action: () => Promise<void>, success: string) => Promise<void>; setStatus: (value: SetupStatus) => void }) {
  return <div className="max-w-2xl"><ConnectionBanner connected={status.authenticated} connectedText="Connected to Roborock Cloud" disconnectedText="Connect your Roborock account to discover devices." /><Field label="Roborock account email" className="mt-5"><input className={inputClass} value={email} onChange={event => setEmail(event.target.value)} placeholder="name@example.com" /></Field><div className="mt-3 flex flex-col gap-2 sm:flex-row"><button disabled={busy || !email} className={primaryButton} onClick={() => void run(() => requestCode(email), 'Verification code sent.')}>Send verification code</button>{!status.authenticated && <><input className={`${inputClass} sm:max-w-48`} placeholder="Verification code" value={code} onChange={event => setCode(event.target.value)} /><button disabled={busy || !code} className={secondaryButton} onClick={() => void run(async () => { await loginWithCode(code); setStatus({ ...status, authenticated: true }); }, 'Roborock account connected.')}>Connect account</button></>}</div></div>;
}

function RobotsStep({ robots, authenticated }: { robots: Array<{ slug: string; name: string; model: string }>; authenticated: boolean }) {
  if (!authenticated) return <EmptyPanel icon={<Bot />} title="Roborock account required" text="Complete the previous step before device discovery." />;
  if (robots.length === 0) return <EmptyPanel icon={<Wifi />} title="Waiting for robots" text="No device has been detected yet. Check the account connection and refresh the setup page." />;
  return <div className="grid gap-3 sm:grid-cols-2">{robots.map(robot => <div key={robot.slug} className="rounded-2xl border border-border bg-background p-4"><div className="flex items-start gap-3"><span className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary"><Bot className="h-5 w-5" /></span><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><strong className="truncate">{robot.name}</strong><Pill tone="green">Ready</Pill></div><p className="mt-1 text-sm text-muted-foreground">{robot.model || 'Unknown model'}</p><code className="mt-2 block truncate text-xs text-muted-foreground">{robot.slug}</code></div></div></div>)}</div>;
}

function ModeStep({ mode, onChange }: { mode: IntegrationMode; onChange: (mode: IntegrationMode) => void }) {
  return <div><div className="grid gap-3 md:grid-cols-3"><ModeCard icon={<Radio />} title="MQTT" text="Compact /core and /activity subscriptions with /command publishing." selected={mode === 'mqtt'} onClick={() => onChange('mqtt')} badge="Mosquitto" /><ModeCard icon={<Cloud />} title="Direct HTTP" text="Push states to Virtual Inputs and receive secured HTTP commands." selected={mode === 'direct'} onClick={() => onChange('direct')} badge="No broker" /><ModeCard icon={<Network />} title="Both" text="Run MQTT and Direct HTTP together for migration or diagnostics." selected={mode === 'both'} onClick={() => onChange('both')} badge="Recommended" /></div><div className="mt-5 rounded-xl bg-muted/60 p-4 text-sm text-muted-foreground"><strong className="text-foreground">Current choice:</strong> {mode === 'mqtt' ? 'MQTT only — a local broker is required.' : mode === 'direct' ? 'Direct HTTP only — no local Mosquitto broker is needed.' : 'Both transports — MQTT and Direct HTTP run independently.'}</div></div>;
}

interface TransportStepProps { status: SetupStatus; payload: any; password: string; busy: boolean; mutate: (fn: (copy: SetupStatus) => void) => void; setPassword: (value: string) => void; run: (action: () => Promise<void>, success: string) => Promise<void>; goToMode: () => void }

function MQTTStep({ status, payload, password, busy, mutate, setPassword, run, goToMode }: TransportStepProps) {
  if (!status.mqtt.enabled) return <DisabledTransport title="MQTT is not active" text="The selected mode does not require Mosquitto." onChange={goToMode} />;
  return <div><ConnectionBanner connected={status.mqtt_diagnostics.connected} connectedText="MQTT broker connected" disconnectedText={status.mqtt_diagnostics.last_error || 'Broker settings have not been verified yet.'} /><div className="mt-5 grid gap-4 md:grid-cols-2"><Field label="Broker URL" className="md:col-span-2"><input className={inputClass} value={status.mqtt.url} onChange={event => mutate(copy => { copy.mqtt.url = event.target.value; })} placeholder="tcp://192.168.1.20:1883" /></Field><Field label="Username"><input className={inputClass} value={status.mqtt.username} onChange={event => mutate(copy => { copy.mqtt.username = event.target.value; })} /></Field><Field label="Password" hint={status.mqtt.password_configured ? 'Already configured — leave blank to keep it.' : undefined}><input type="password" className={inputClass} value={password} onChange={event => setPassword(event.target.value)} placeholder={status.mqtt.password_configured ? '••••••••••••' : ''} /></Field><Field label="Base topic"><input className={inputClass} value={status.mqtt.topic} onChange={event => mutate(copy => { copy.mqtt.topic = event.target.value; })} /></Field><Toggle title="Use TLS" description="Encrypt the connection to the MQTT broker." checked={status.mqtt.tls} onChange={value => mutate(copy => { copy.mqtt.tls = value; })} /></div><button disabled={busy} className={`${secondaryButton} mt-5`} onClick={() => void run(() => testMQTTSettings(payload.mqtt), 'MQTT connection succeeded.')}>Test MQTT connection</button></div>;
}

function DirectStep({ status, payload, password, token, busy, mutate, setPassword, setToken, run, goToMode }: TransportStepProps & { token: string; setToken: (value: string) => void }) {
  if (!status.loxone.direct.enabled) return <DisabledTransport title="Direct HTTP is not active" text="The selected mode currently uses MQTT only." onChange={goToMode} />;
  return <div><ConnectionBanner connected={Boolean(status.direct_diagnostics?.last_success_at)} connectedText="Miniserver transmission succeeded" disconnectedText={status.direct_diagnostics?.last_error || 'Test the Miniserver connection before finishing.'} /><div className="mt-5 grid gap-4 md:grid-cols-2"><Field label="Miniserver address"><input className={inputClass} value={status.loxone.direct.host} onChange={event => mutate(copy => { copy.loxone.direct.host = event.target.value; })} placeholder="192.168.1.10" /></Field><Field label="Port"><input type="number" className={inputClass} value={status.loxone.direct.port} onChange={event => mutate(copy => { copy.loxone.direct.port = Number(event.target.value); })} /></Field><Field label="Miniserver username"><input className={inputClass} value={status.loxone.direct.username} onChange={event => mutate(copy => { copy.loxone.direct.username = event.target.value; })} /></Field><Field label="Miniserver password" hint={status.loxone.direct.password_configured ? 'Already configured — leave blank to keep it.' : undefined}><input type="password" className={inputClass} value={password} onChange={event => setPassword(event.target.value)} placeholder={status.loxone.direct.password_configured ? '••••••••••••' : ''} /></Field><Field label="Virtual Input prefix"><input className={inputClass} value={status.loxone.direct.input_prefix} onChange={event => mutate(copy => { copy.loxone.direct.input_prefix = event.target.value; })} /></Field><Field label="Allowed command CIDRs" hint="Restrict commands to the Miniserver network."><input className={inputClass} value={(status.loxone.direct.allowed_cidrs ?? []).join(', ')} onChange={event => mutate(copy => { copy.loxone.direct.allowed_cidrs = event.target.value.split(',').map(value => value.trim()).filter(Boolean); })} placeholder="192.168.1.0/24" /></Field></div><div className="mt-5 grid gap-3 sm:grid-cols-2"><button disabled={busy} className={secondaryButton} onClick={() => void run(() => testDirectSettings(payload.loxone.direct), 'Miniserver connection succeeded.')}>Test Miniserver connection</button><button disabled={busy} className={secondaryButton} onClick={() => void run(async () => { const next = await rotateDirectToken(); setToken(next); }, 'New API token generated. Copy it now.')}>Rotate command token</button></div>{token && <div className="mt-4 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm"><div className="flex items-center gap-2 font-semibold text-amber-800 dark:text-amber-200"><KeyRound className="h-4 w-4" /> One-time command token</div><code className="mt-2 block break-all rounded-lg bg-background/70 p-3 text-xs">{token}</code><p className="mt-2 text-xs text-muted-foreground">Copy it now. The stored token will not be displayed again.</p></div>}</div>;
}

function ReviewStep({ status, robots, mode }: { status: SetupStatus; robots: Array<{ slug: string; name: string; model: string }>; mode: IntegrationMode }) {
  return <div><div className="grid gap-3 md:grid-cols-3"><ReviewCard title="Roborock" value={status.authenticated ? `${robots.length} connected robot${robots.length === 1 ? '' : 's'}` : 'Account required'} ok={status.authenticated} /><ReviewCard title="Integration mode" value={mode === 'both' ? 'MQTT + Direct HTTP' : mode === 'direct' ? 'Direct HTTP' : 'MQTT'} ok /><ReviewCard title="Security" value={status.loxone.direct.enabled ? status.loxone.direct.api_token_configured ? 'Command token configured' : 'Command token required' : 'Direct API disabled'} ok={!status.loxone.direct.enabled || status.loxone.direct.api_token_configured} /></div><div className="mt-5 rounded-2xl border border-border bg-background p-5"><h3 className="font-semibold">What happens when you save</h3><ul className="mt-3 space-y-2 text-sm text-muted-foreground"><li className="flex gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /> Settings are written to the persistent data volume.</li><li className="flex gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /> Enabled transports are reconfigured without rebuilding Docker.</li><li className="flex gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /> Current robot state is resynchronized without creating command activity.</li></ul></div></div>;
}

function FinishStep({ canFinish, busy, onFinish }: { canFinish: boolean; busy: boolean; onFinish: () => void }) {
  return <div className="mx-auto max-w-xl py-6 text-center"><span className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-green-500/15 text-green-600"><ShieldCheck className="h-8 w-8" /></span><h3 className="mt-5 text-2xl font-bold">Ready to apply</h3><p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-muted-foreground">Your settings will be persisted and the selected integrations will start immediately.</p>{!canFinish && <p className="mt-4 rounded-xl bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300">Connect Roborock and enable at least one integration mode before finishing.</p>}<button disabled={busy || !canFinish} className={`${primaryButton} mt-6 px-6`} onClick={onFinish}>{busy ? 'Applying configuration…' : 'Finish and start bridge'}</button></div>;
}

function StatusCard({ icon, title, value, ok, muted = false }: { icon: ReactNode; title: string; value: string; ok: boolean; muted?: boolean }) { return <div className="flex min-w-0 items-center gap-3 rounded-2xl border border-border bg-card/90 p-3.5 shadow-sm backdrop-blur"><span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${muted ? 'bg-muted text-muted-foreground' : ok ? 'bg-green-500/15 text-green-600' : 'bg-amber-500/15 text-amber-600'}`}>{icon}</span><div className="min-w-0"><p className="text-xs font-medium text-muted-foreground">{title}</p><p className="truncate text-sm font-semibold" title={value}>{value}</p></div></div>; }
function ModeCard({ icon, title, text, badge, selected, onClick }: { icon: ReactNode; title: string; text: string; badge: string; selected: boolean; onClick: () => void }) { return <button type="button" onClick={onClick} className={`relative rounded-2xl border p-4 text-left transition-all ${selected ? 'border-primary bg-primary/5 shadow-md ring-1 ring-primary/30' : 'border-border bg-background hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-sm'}`}><div className="flex items-start justify-between gap-2"><span className={`flex h-10 w-10 items-center justify-center rounded-xl ${selected ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>{icon}</span>{selected ? <Pill tone="green"><Check className="h-3 w-3" /> Selected</Pill> : <Pill>{badge}</Pill>}</div><strong className="mt-4 block">{title}</strong><span className="mt-1 block text-xs leading-relaxed text-muted-foreground">{text}</span></button>; }
function ConnectionBanner({ connected, connectedText, disconnectedText }: { connected: boolean; connectedText: string; disconnectedText: string }) { return <div className={`flex items-center gap-3 rounded-xl border p-3 text-sm ${connected ? 'border-green-500/25 bg-green-500/10' : 'border-amber-500/25 bg-amber-500/10'}`}><span className={`h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-amber-500'}`} /><span><strong>{connected ? connectedText : 'Attention required'}</strong>{!connected && <span className="ml-2 text-muted-foreground">{disconnectedText}</span>}</span></div>; }
function Feature({ icon, title, text }: { icon: ReactNode; title: string; text: string }) { return <div className="rounded-xl border border-border bg-background p-4"><span className="text-primary">{icon}</span><strong className="mt-3 block text-sm">{title}</strong><span className="mt-1 block text-xs leading-relaxed text-muted-foreground">{text}</span></div>; }
function Field({ label, hint, className = '', children }: { label: string; hint?: string; className?: string; children: ReactNode }) { return <label className={`block text-sm ${className}`}><span className="font-medium">{label}</span>{children}{hint && <span className="mt-1 block text-xs text-muted-foreground">{hint}</span>}</label>; }
function Toggle({ title, description, checked, onChange }: { title: string; description?: string; checked: boolean; onChange: (value: boolean) => void }) { return <label className="flex cursor-pointer items-center gap-3 rounded-xl border border-border bg-background p-3"><span className="min-w-0 flex-1"><strong className="block text-sm">{title}</strong>{description && <span className="block text-xs text-muted-foreground">{description}</span>}</span><input type="checkbox" checked={checked} onChange={event => onChange(event.target.checked)} className="h-5 w-5" /></label>; }
function ReviewCard({ title, value, ok }: { title: string; value: string; ok: boolean }) { return <div className="rounded-xl border border-border bg-background p-4"><div className="flex items-center justify-between gap-2"><span className="text-xs text-muted-foreground">{title}</span><span className={`h-2 w-2 rounded-full ${ok ? 'bg-green-500' : 'bg-amber-500'}`} /></div><strong className="mt-2 block text-sm">{value}</strong></div>; }
function EmptyPanel({ icon, title, text }: { icon: ReactNode; title: string; text: string }) { return <div className="rounded-2xl border border-dashed border-border p-8 text-center"><span className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">{icon}</span><h3 className="mt-3 font-semibold">{title}</h3><p className="mt-1 text-sm text-muted-foreground">{text}</p></div>; }
function DisabledTransport({ title, text, onChange }: { title: string; text: string; onChange: () => void }) { return <div className="rounded-2xl border border-dashed border-border p-8 text-center"><Server className="mx-auto h-9 w-9 text-muted-foreground" /><h3 className="mt-3 font-semibold">{title}</h3><p className="mt-1 text-sm text-muted-foreground">{text}</p><button onClick={onChange} className={`${secondaryButton} mt-4`}>Change integration mode</button></div>; }
function Pill({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'blue' | 'green' }) { const styles = tone === 'blue' ? 'border-blue-500/25 bg-blue-500/10 text-blue-600 dark:text-blue-400' : tone === 'green' ? 'border-green-500/25 bg-green-500/10 text-green-600 dark:text-green-400' : 'border-border bg-muted text-muted-foreground'; return <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium ${styles}`}>{children}</span>; }

const inputClass = 'mt-1.5 w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm outline-none transition-shadow focus:ring-2 focus:ring-ring';
const primaryButton = 'touch-target inline-flex items-center justify-center rounded-xl bg-primary px-4 text-sm font-medium text-primary-foreground shadow-sm hover:opacity-90 disabled:opacity-50';
const secondaryButton = 'touch-target inline-flex items-center justify-center rounded-xl border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:opacity-50';
