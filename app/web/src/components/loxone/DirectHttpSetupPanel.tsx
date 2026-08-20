import { ChevronDown, CircleHelp, KeyRound, Link2, Network, ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { directCommandURL, directConnectorAddress, normalizeBridgeAddress, suggestedLoxoneInputLabel } from '@/lib/loxone';
import type { LoxoneDirectInput, LoxoneDirectOutput, LoxoneRobot } from '@/types/loxone';
import { CopyButton, EmptyState, StatusBadge } from './LoxoneUI';

interface Props {
  robot: LoxoneRobot;
  apiUsername: string;
  tokenConfigured: boolean;
}

const inputLabels: Record<string, string> = {
  online: 'Roborock cloud transport connected', robot_online: 'Robot responding to status polling', battery: 'Battery percentage', state: 'Stable numeric state code',
  state_text: 'Readable robot state', current_room_id: 'Current segment ID', current_room_name: 'Current room name',
  active_program: 'Current cleaning program type', active_scene_id: 'Current bridge-triggered scene ID', active_scene_name: 'Current bridge-triggered scene name',
  error_code: 'Roborock error code', error_text: 'Readable error', clean_area: 'Cleaned area in m²',
  clean_time_seconds: 'Cleaning time in seconds', last_seen: 'Last update as Unix timestamp',
  main_brush: 'Main brush remaining', side_brush: 'Side brush remaining', filter: 'Filter remaining',
  sensor: 'Sensors remaining', dock_type: 'Reported dock type', charge_status: 'Charging status',
  dock_error_status: 'Dock error status', dust_collection_status: 'Auto-empty status',
  wash_status: 'Mop washing status', dry_status: 'Mop drying status',
};

export function DirectHttpSetupPanel({ robot, apiUsername, tokenConfigured }: Props) {
  const [bridgeAddress, setBridgeAddress] = useState(() => typeof window === 'undefined' ? 'http://SYNOLOGY-IP:8080' : window.location.origin);
  const [token, setToken] = useState('');
  const address = useMemo(() => {
    try { return { value: normalizeBridgeAddress(bridgeAddress), error: '' }; }
    catch (error) { return { value: '', error: error instanceof Error ? error.message : 'Invalid bridge address.' }; }
  }, [bridgeAddress]);
  const tokenReady = token.length >= 32;
  const connectorAddress = address.value ? directConnectorAddress(address.value, apiUsername, tokenReady ? token : '') : '';
  const connectorPreview = connectorAddress && !tokenReady ? directConnectorAddress(address.value, apiUsername, 'YOUR_API_TOKEN') : connectorAddress;
  const inputs = robot.direct_inputs ?? [];
  const outputs = robot.direct_outputs ?? [];
  const standard = outputs.filter(output => ['start', 'pause', 'dock'].includes(output.command));
  const rooms = outputs.filter(output => output.path.includes('/rooms/'));
  const scenes = outputs.filter(output => output.path.includes('/scenes/'));
  const advanced = outputs.filter(output => !standard.includes(output) && !rooms.includes(output) && !scenes.includes(output));

  if (!robot.direct_enabled) return <div className="rounded-xl border border-dashed border-border p-4 text-sm">
    <div className="flex items-center gap-2 font-medium"><Network className="h-4 w-4" /> Direct HTTP is disabled for this robot</div>
    <p className="mt-1 text-muted-foreground">Enable Direct Loxone Integration to display the exact Virtual Input names and Virtual Output commands.</p>
    <Link to="/setup" className="mt-3 inline-flex rounded-lg border border-border px-3 py-2 hover:bg-accent">Open integration settings</Link>
  </div>;

  return <details className="group overflow-hidden rounded-xl border border-border bg-background">
    <summary className="flex cursor-pointer list-none items-center gap-3 p-4 hover:bg-accent/50">
      <div className="rounded-lg bg-primary/10 p-2 text-primary"><Link2 className="h-5 w-5" /></div>
      <div className="min-w-0 flex-1"><strong className="block">Ready-to-copy Loxone Config setup</strong><span className="text-xs text-muted-foreground">Exact input names, connector address and HTTP commands for {robot.name}</span></div>
      <StatusBadge tone="green">Direct</StatusBadge><ChevronDown className="h-5 w-5 text-muted-foreground transition-transform group-open:rotate-180" />
    </summary>
    <div className="space-y-5 border-t border-border p-4 md:p-5">
      <div className="rounded-xl border border-blue-500/30 bg-blue-500/5 p-4 text-sm">
        <strong>How it works</strong>
        <p className="mt-1 text-muted-foreground">Create the inputs once, then create one authenticated Virtual Output connector. The bridge pushes state changes into Loxone; Loxone sends commands back through that connector.</p>
      </div>

      <SetupStep number="1" title="Virtual Output connector" description="In Loxone Config: Periphery → Virtual Outputs → add a Virtual Output connector.">
        <div className="grid gap-3 lg:grid-cols-2">
          <label className="text-sm"><span className="font-medium">Bridge address visible from Loxone</span><input value={bridgeAddress} onChange={event => setBridgeAddress(event.target.value)} className={`mt-1 w-full rounded-lg border bg-card px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-ring ${address.error ? 'border-red-500' : 'border-border'}`} placeholder="http://192.168.1.20:8080" />{address.error && <span className="mt-1 block text-xs text-red-500">{address.error}</span>}</label>
          <label className="text-sm"><span className="flex items-center gap-1 font-medium">Command API token <span title="Kept only in this browser tab. It is never sent back to the bridge or stored."><CircleHelp className="h-3.5 w-3.5 text-muted-foreground" /></span></span><input type="password" autoComplete="off" value={token} onChange={event => setToken(event.target.value.trim())} className={`mt-1 w-full rounded-lg border bg-card px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-ring ${token && !tokenReady ? 'border-amber-500' : 'border-border'}`} placeholder="Paste the one-time token here" /><span className="mt-1 block text-xs text-muted-foreground">Not saved. {tokenConfigured ? 'A token is configured on the bridge.' : 'No token is configured yet.'}</span></label>
        </div>
        {!tokenReady && <div className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-800 dark:text-amber-200"><KeyRound className="mr-2 inline h-4 w-4" />Paste the token shown when it was generated. If it is lost, rotate it in <Link to="/setup" className="font-medium underline">Modes &amp; settings → Direct HTTP</Link>; rotating invalidates the previous connector password.</div>}
        <CopyField label="Address" value={connectorPreview} copyEnabled={tokenReady && !address.error} help="Paste this complete authenticated address into the Virtual Output connector’s Address field." />
        <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground"><StatusBadge tone="blue">Basic authentication</StatusBadge><span>User: <code>{apiUsername}</code></span><span>Protocol: HTTP{address.value.startsWith('https:') ? 'S' : ''}</span></div>
      </SetupStep>

      <SetupStep number="2" title={`Virtual Inputs (${inputs.length})`} description="Keep the exact input name for HTTP updates. Copy the suggested label into a separate display/description field when available; do not replace the exact name unless you also configure an input override.">
        {inputs.length === 0 ? <EmptyState>No Direct input plan is available yet. Refresh after the robot has reported its status.</EmptyState> : <details className="rounded-lg border border-border">
          <summary className="cursor-pointer px-3 py-3 text-sm font-medium">Show the complete input list</summary>
          <div className="divide-y divide-border border-t border-border">{inputs.map(input => <InputRow key={`${input.field}:${input.name}`} input={input} robotName={robot.name} />)}</div>
        </details>}
        <div className="mt-3 grid gap-2 sm:grid-cols-3"><TypeHint kind="digital" text="Virtual Input · digital" /><TypeHint kind="analog" text="Virtual Input · analog" /><TypeHint kind="text" text="Virtual Text Input" /></div>
      </SetupStep>

      <SetupStep number="3" title={`Virtual Output commands (${outputs.length})`} description="Under the connector above, add one Virtual Output Command for each action you need. Use the path in “Command when ON” and select POST.">
        <CommandGroup title="Essential controls" outputs={standard} connector={connectorAddress} ready={tokenReady && !address.error} open />
        <CommandGroup title={`Rooms (${rooms.length})`} outputs={rooms} connector={connectorAddress} ready={tokenReady && !address.error} />
        <CommandGroup title={`Scenes (${scenes.length})`} outputs={scenes} connector={connectorAddress} ready={tokenReady && !address.error} />
        <CommandGroup title={`Supported advanced controls (${advanced.length})`} outputs={advanced} connector={connectorAddress} ready={tokenReady && !address.error} />
      </SetupStep>

      <div className="flex items-start gap-2 rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /><span>The token is displayed only in this tab and never included in exports or logs. Prefer HTTPS when available; on HTTP, restrict the command API to the Miniserver CIDR.</span></div>
    </div>
  </details>;
}

function SetupStep({ number, title, description, children }: { number: string; title: string; description: string; children: React.ReactNode }) {
  return <section><div className="mb-3 flex items-start gap-3"><span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary text-sm font-semibold text-primary-foreground">{number}</span><div><h4 className="font-semibold">{title}</h4><p className="text-xs text-muted-foreground">{description}</p></div></div><div className="md:pl-10">{children}</div></section>;
}

function CopyField({ label, value, copyEnabled, help }: { label: string; value: string; copyEnabled: boolean; help?: string }) {
  return <div className="mt-3"><div className="mb-1 flex items-center justify-between gap-2"><span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</span>{help && <span className="hidden text-xs text-muted-foreground lg:block">{help}</span>}</div><div className="flex min-w-0 gap-2"><code className="min-w-0 flex-1 overflow-x-auto rounded-lg border border-border bg-muted/50 px-3 py-2.5 text-xs whitespace-nowrap">{value || 'Enter a valid bridge address'}</code><CopyButton value={value} disabled={!copyEnabled} label={`Copy ${label}`} /></div>{help && <p className="mt-1 text-xs text-muted-foreground lg:hidden">{help}</p>}</div>;
}

function InputRow({ input, robotName }: { input: LoxoneDirectInput; robotName: string }) {
  const suggestedLabel = suggestedLoxoneInputLabel(robotName, input.field);
  return <div className="grid gap-3 p-3 sm:grid-cols-[100px_1fr] sm:items-start"><StatusBadge tone={input.kind === 'digital' ? 'green' : input.kind === 'text' ? 'blue' : 'neutral'}>{input.kind}</StatusBadge><div className="min-w-0 space-y-2"><div className="flex min-w-0 items-center gap-2"><div className="min-w-0 flex-1"><span className="block text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Exact input name</span><code className="block truncate text-xs font-semibold" title={input.name}>{input.name}</code><span className="text-xs text-muted-foreground">{inputLabels[input.field] ?? input.field}</span></div><CopyButton value={input.name} label={`Copy exact input name ${input.name}`} /></div><div className="flex min-w-0 items-center gap-2 rounded-lg bg-muted/60 px-3 py-2"><div className="min-w-0 flex-1"><span className="block text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Suggested label</span><span className="block truncate text-sm font-medium" title={suggestedLabel}>{suggestedLabel}</span></div><CopyButton value={suggestedLabel} label={`Copy suggested label ${suggestedLabel}`} /></div></div></div>;
}

function TypeHint({ kind, text }: { kind: LoxoneDirectInput['kind']; text: string }) {
  return <div className="rounded-lg bg-muted/60 px-3 py-2 text-xs"><StatusBadge tone={kind === 'digital' ? 'green' : kind === 'text' ? 'blue' : 'neutral'}>{kind}</StatusBadge><span className="ml-2">{text}</span></div>;
}

function CommandGroup({ title, outputs, connector, ready, open = false }: { title: string; outputs: LoxoneDirectOutput[]; connector: string; ready: boolean; open?: boolean }) {
  if (outputs.length === 0) return null;
  return <details open={open} className="mb-2 overflow-hidden rounded-lg border border-border"><summary className="cursor-pointer bg-muted/40 px-3 py-2.5 text-sm font-medium">{title}</summary><div className="divide-y divide-border border-t border-border">{outputs.map(output => <div key={output.path} className="p-3"><div className="flex flex-wrap items-center gap-2"><strong className="text-sm">{output.name}</strong><StatusBadge tone="blue">POST</StatusBadge>{output.level === 'advanced' && <StatusBadge tone="amber">Advanced</StatusBadge>}</div><div className="mt-2 grid gap-2 xl:grid-cols-2"><CopyField label="Command when ON" value={output.path} copyEnabled help="Paste under the Virtual Output Command." /><CopyField label="Full URL" value={connector ? directCommandURL(connector, output.path) : output.path} copyEnabled={ready} help="Useful for checking the complete target." /></div></div>)}</div></details>;
}
