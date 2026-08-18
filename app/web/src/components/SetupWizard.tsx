import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Check, ChevronLeft, ChevronRight, Cloud, Network, Radio, ShieldCheck } from 'lucide-react';
import {
  fetchDevices, fetchSetupStatus, loginWithCode, requestCode, rotateDirectToken,
  saveSetupSettings, testDirectSettings, testMQTTSettings,
} from '@/lib/api';
import type { SetupStatus } from '@/lib/api';

const steps = ['Welcome', 'Roborock', 'Robots', 'Mode', 'MQTT', 'Direct HTTP', 'Review', 'Finish'];

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

  if (!status || !payload) return <div className="min-h-screen bg-background flex items-center justify-center text-muted-foreground">Loading setup…</div>;

  const mutate = (fn: (copy: SetupStatus) => void) => {
    const copy = structuredClone(status); fn(copy); setStatus(copy); setMessage(null);
  };
  const save = async (complete = false) => {
    setBusy(true); setMessage(null);
    try {
      const saved = await saveSetupSettings({ ...payload, setup_complete: complete });
      setStatus(saved); setMQTTPassword(''); setDirectPassword('');
      setMessage({ ok: true, text: complete ? 'Configuration complete.' : 'Settings saved and applied.' });
      if (complete) onComplete();
    } catch (error) { setMessage({ ok: false, text: error instanceof Error ? error.message : 'Save failed' }); }
    finally { setBusy(false); }
  };
  const run = async (action: () => Promise<void>, success: string) => {
    setBusy(true); setMessage(null);
    try { await action(); setMessage({ ok: true, text: success }); }
    catch (error) { setMessage({ ok: false, text: error instanceof Error ? error.message : 'Operation failed' }); }
    finally { setBusy(false); }
  };

  const field = 'w-full rounded-lg border border-border bg-background px-3 py-2 text-sm';
  return <div className="min-h-screen bg-background p-4 md:p-10">
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-center gap-3"><div className="rounded-xl bg-primary/10 p-3"><Network className="h-6 w-6 text-primary" /></div><div><h1 className="text-2xl font-bold">roborock-mqtt-loxone</h1><p className="text-sm text-muted-foreground">Integration Setup Wizard</p></div></div>
      <div className="mb-5 grid grid-cols-4 gap-1 md:grid-cols-8">{steps.map((name, index) => <button key={name} onClick={() => setStep(index)} className={`rounded-md px-1 py-2 text-xs ${index === step ? 'bg-primary text-primary-foreground' : index < step ? 'bg-green-500/15 text-green-600' : 'bg-muted text-muted-foreground'}`}>{index < step ? <Check className="mx-auto h-3 w-3" /> : index + 1}<span className="hidden md:block">{name}</span></button>)}</div>
      <section className="rounded-2xl border border-border bg-card p-5 shadow-sm md:p-7">
        {step === 0 && <div><h2 className="text-xl font-semibold">Welcome</h2><p className="mt-3 text-muted-foreground">This wizard stores integration settings in the mounted data volume. MQTT, Direct Loxone, or both can be enabled independently. Roborock Cloud access never depends on your local Mosquitto broker.</p><div className="mt-5 rounded-lg bg-amber-500/10 p-3 text-sm">Back up the data volume: it contains the Roborock session and encrypted-network-bound integration credentials with owner-only file permissions.</div></div>}
        {step === 1 && <div><h2 className="text-xl font-semibold">Roborock account</h2><label className="mt-4 block text-sm">Email<input className={`${field} mt-1`} value={email} onChange={e => setEmail(e.target.value)} /></label><div className="mt-3 flex flex-wrap gap-2"><button disabled={busy || !email} className="rounded-lg bg-primary px-4 py-2 text-primary-foreground" onClick={() => run(() => requestCode(email), 'Verification code sent.')}>Send code</button>{!status.authenticated && <><input className={`${field} max-w-40`} placeholder="Verification code" value={code} onChange={e => setCode(e.target.value)} /><button disabled={busy || !code} className="rounded-lg border px-4 py-2" onClick={() => run(async () => { await loginWithCode(code); setStatus({ ...status, authenticated: true }); }, 'Roborock account connected.')}>Connect</button></>}</div>{status.authenticated && <p className="mt-3 text-sm text-green-600">Connected to Roborock Cloud.</p>}</div>}
        {step === 2 && <div><h2 className="text-xl font-semibold">Detected robots</h2>{robots.length === 0 ? <p className="mt-3 text-muted-foreground">Authenticate first, then device discovery will populate this list.</p> : <div className="mt-4 space-y-2">{robots.map(robot => <div key={robot.slug} className="rounded-lg border p-3"><strong>{robot.name}</strong><div className="text-sm text-muted-foreground">{robot.model} · stable slug: {robot.slug}</div></div>)}</div>}</div>}
        {step === 3 && <div><h2 className="text-xl font-semibold">Integration mode</h2><p className="mt-2 text-sm text-muted-foreground">Select either transport or both. Direct-only requires no local MQTT broker.</p><div className="mt-4 grid gap-3 md:grid-cols-2"><Toggle icon={<Radio />} title="MQTT Integration" checked={status.mqtt.enabled} onChange={v => mutate(s => { s.mqtt.enabled = v; s.loxone.enabled = v; })} /><Toggle icon={<Cloud />} title="Direct Loxone Integration" checked={status.loxone.direct.enabled} onChange={v => mutate(s => { s.loxone.direct.enabled = v; })} /></div></div>}
        {step === 4 && <div><h2 className="text-xl font-semibold">Local MQTT broker</h2>{!status.mqtt.enabled ? <p className="mt-3 text-muted-foreground">MQTT is disabled; Mosquitto is not required.</p> : <div className="mt-4 grid gap-3 md:grid-cols-2"><label className="text-sm md:col-span-2">Broker URL<input className={`${field} mt-1`} value={status.mqtt.url} onChange={e => mutate(s => { s.mqtt.url = e.target.value; })} placeholder="tcp://192.168.1.20:1883" /></label><label className="text-sm">Username<input className={`${field} mt-1`} value={status.mqtt.username} onChange={e => mutate(s => { s.mqtt.username = e.target.value; })} /></label><label className="text-sm">Password<input type="password" className={`${field} mt-1`} value={mqttPassword} onChange={e => setMQTTPassword(e.target.value)} placeholder={status.mqtt.password_configured ? 'Configured — leave blank to keep' : ''} /></label><label className="text-sm">Base topic<input className={`${field} mt-1`} value={status.mqtt.topic} onChange={e => mutate(s => { s.mqtt.topic = e.target.value; })} /></label><Toggle title="TLS" checked={status.mqtt.tls} onChange={v => mutate(s => { s.mqtt.tls = v; })} /><button disabled={busy} className="rounded-lg border px-4 py-2" onClick={() => run(() => testMQTTSettings(payload.mqtt), 'MQTT loopback succeeded.')}>Test connection</button></div>}</div>}
        {step === 5 && <div><h2 className="text-xl font-semibold">Direct Loxone</h2>{!status.loxone.direct.enabled ? <p className="mt-3 text-muted-foreground">Direct HTTP is disabled.</p> : <div className="mt-4 grid gap-3 md:grid-cols-2"><label className="text-sm">Miniserver address<input className={`${field} mt-1`} value={status.loxone.direct.host} onChange={e => mutate(s => { s.loxone.direct.host = e.target.value; })} placeholder="192.168.1.10" /></label><label className="text-sm">Port<input type="number" className={`${field} mt-1`} value={status.loxone.direct.port} onChange={e => mutate(s => { s.loxone.direct.port = Number(e.target.value); })} /></label><label className="text-sm">Miniserver username<input className={`${field} mt-1`} value={status.loxone.direct.username} onChange={e => mutate(s => { s.loxone.direct.username = e.target.value; })} /></label><label className="text-sm">Miniserver password<input type="password" className={`${field} mt-1`} value={directPassword} onChange={e => setDirectPassword(e.target.value)} placeholder={status.loxone.direct.password_configured ? 'Configured — leave blank to keep' : ''} /></label><label className="text-sm">Virtual Input prefix<input className={`${field} mt-1`} value={status.loxone.direct.input_prefix} onChange={e => mutate(s => { s.loxone.direct.input_prefix = e.target.value; })} /></label><label className="text-sm">Allowed command CIDRs<input className={`${field} mt-1`} value={(status.loxone.direct.allowed_cidrs ?? []).join(', ')} onChange={e => mutate(s => { s.loxone.direct.allowed_cidrs = e.target.value.split(',').map(x => x.trim()).filter(Boolean); })} placeholder="192.168.1.0/24" /></label><button disabled={busy} className="rounded-lg border px-4 py-2" onClick={() => run(() => testDirectSettings(payload.loxone.direct), 'Miniserver connection succeeded.')}>Test connection</button><button disabled={busy} className="rounded-lg border px-4 py-2" onClick={() => run(async () => { const next = await rotateDirectToken(); setToken(next); }, 'New API token generated. Copy it now.')}>Rotate command token</button>{token && <div className="md:col-span-2 rounded-lg bg-amber-500/10 p-3 text-sm break-all"><strong>One-time token:</strong> {token}</div>}</div>}</div>}
        {step === 6 && <div><h2 className="text-xl font-semibold">Review</h2><div className="mt-4 grid gap-3 md:grid-cols-3"><Summary title="Roborock" value={status.authenticated ? `${robots.length} robot(s)` : 'Not connected'} /><Summary title="MQTT" value={status.mqtt.enabled ? status.mqtt.url || 'Address required' : 'Disabled'} /><Summary title="Direct HTTP" value={status.loxone.direct.enabled ? status.loxone.direct.host || 'Address required' : 'Disabled'} /></div><p className="mt-4 text-sm text-muted-foreground">Capabilities, commandable rooms and scenes are discovered from each robot. Unsupported or unverified controls remain hidden.</p><button disabled={busy} className="mt-4 rounded-lg border px-4 py-2" onClick={() => save(false)}>Save and apply now</button></div>}
        {step === 7 && <div className="text-center"><ShieldCheck className="mx-auto h-12 w-12 text-green-500" /><h2 className="mt-3 text-xl font-semibold">Ready to finish</h2><p className="mt-2 text-muted-foreground">Settings will be persisted and both transports reconfigured live. Current state is republished without generating command activity.</p><button disabled={busy || !status.authenticated || (!status.mqtt.enabled && !status.loxone.direct.enabled)} className="mt-5 rounded-lg bg-primary px-5 py-2.5 text-primary-foreground disabled:opacity-50" onClick={() => save(true)}>{busy ? 'Applying…' : 'Finish setup'}</button></div>}
        {message && <div className={`mt-5 rounded-lg p-3 text-sm ${message.ok ? 'bg-green-500/10 text-green-600' : 'bg-red-500/10 text-red-600'}`}>{message.text}</div>}
      </section>
      <div className="mt-4 flex justify-between"><button disabled={step === 0} className="flex items-center gap-1 rounded-lg px-3 py-2 disabled:opacity-30" onClick={() => setStep(x => Math.max(0, x - 1))}><ChevronLeft className="h-4 w-4" /> Back</button>{step < steps.length - 1 && <button className="flex items-center gap-1 rounded-lg bg-primary px-3 py-2 text-primary-foreground" onClick={() => setStep(x => Math.min(steps.length - 1, x + 1))}>Next <ChevronRight className="h-4 w-4" /></button>}</div>
    </div>
  </div>;
}

function Toggle({ icon, title, checked, onChange }: { icon?: ReactNode; title: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="flex cursor-pointer items-center gap-3 rounded-lg border p-3">{icon}<span className="flex-1 text-sm font-medium">{title}</span><input type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)} className="h-5 w-5" /></label>;
}

function Summary({ title, value }: { title: string; value: string }) { return <div className="rounded-lg bg-muted p-3"><div className="text-xs text-muted-foreground">{title}</div><div className="mt-1 font-medium break-all">{value}</div></div>; }
