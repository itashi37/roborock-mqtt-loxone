import { useState, useEffect, useMemo } from 'react';
import { Routes, Route, Navigate, useParams, useNavigate, Link } from 'react-router-dom';
import { Battery, Sun, Moon, Wifi, WifiOff, Play, Pause, Home, Wind, Droplets, AlertCircle, Clock, MapPin, LogOut, ChevronRight, Wrench, Network, ServerCog } from 'lucide-react';
import { useSSE } from '@/hooks/useSSE';
import { pauseCleaning, dockVacuum, getAuthStatus, logout, fetchDevices, fetchScenes, executeScene, setNotAtHome, fetchSetupStatus } from '@/lib/api';
import type { SceneInfo } from '@/lib/api';
import { useTheme } from '@/contexts/ThemeContext';
import { formatCleanTime, formatCleanArea, formatDisplayName } from '@/types/status';
import type { DeviceSummary } from '@/types/status';
import { LoginPage } from '@/components/LoginPage';
import { DeviceSwitcher } from '@/components/DeviceSwitcher';
import { CleaningProgress } from '@/components/CleaningProgress';
import { VectorMap } from '@/components/VectorMap';
import { ScheduleSection } from '@/components/ScheduleSection';
import { ConfirmModal } from '@/components/ConfirmModal';
import { ControlsPage } from '@/components/ControlsPage';
import { MaintenancePage } from '@/components/MaintenancePage';
import { SchedulePage } from '@/components/SchedulePage';
import { LoxoneIntegrationPage } from '@/components/LoxoneIntegrationPage';
import { SetupWizard } from '@/components/SetupWizard';
import { SystemUpdatesPage } from '@/components/SystemUpdatesPage';

const activeCleaningStates = new Set([
  'cleaning', 'spot_cleaning', 'segment_cleaning', 'zoned_cleaning',
  'going_to_target', 'returning_home', 'washing_mop', 'emptying_dustbin',
  'going_to_wash_mop', 'paused',
]);

export function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [setupComplete, setSetupComplete] = useState<boolean | null>(null);
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const { statuses, scheduleStates, availabilities, loxoneActivities, isConnected, error, reconnect } = useSSE();
  const { theme, toggleTheme } = useTheme();
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [scenesBySlug, setScenesBySlug] = useState<Record<string, SceneInfo[]>>({});
  const [activeSceneId, setActiveSceneId] = useState<number | null>(null);
  const [showLogoutModal, setShowLogoutModal] = useState(false);
  const [pendingScene, setPendingScene] = useState<SceneInfo | null>(null);
  const [pendingSceneSlug, setPendingSceneSlug] = useState<string>('');
  const [globalNotAtHome, setGlobalNotAtHome] = useState(false);

  useEffect(() => {
    Promise.all([fetchSetupStatus(), getAuthStatus()])
      .then(([setup, auth]) => { setSetupComplete(setup.setup_complete); setAuthenticated(auth.authenticated); })
      .catch(() => { setSetupComplete(false); setAuthenticated(false); });
  }, []);

  useEffect(() => {
    if (!authenticated) return;
    fetchDevices().then(devs => {
      setDevices(devs);
      devs.forEach(dev => {
        fetchScenes(dev.slug).then(scenes => {
          setScenesBySlug(prev => ({ ...prev, [dev.slug]: scenes }));
        }).catch(() => {});
      });
    }).catch(console.error);
  }, [authenticated]);

  useEffect(() => {
    const states = Object.values(scheduleStates);
    if (states.length > 0) {
      setGlobalNotAtHome(states[0].not_at_home);
    }
  }, [scheduleStates]);

  // Overlay the live SSE connection state onto the fetched device list so the
  // online/offline indicator reflects cloud-connection drops without a refetch.
  const liveDevices = useMemo(
    () => devices.map(d => (d.slug in availabilities ? { ...d, online: availabilities[d.slug] } : d)),
    [devices, availabilities],
  );

  const handleAction = async (action: string, fn: () => Promise<void>) => {
    setActionLoading(action);
    try {
      await fn();
    } catch (err) {
      console.error(`Failed to ${action}:`, err);
    } finally {
      setTimeout(() => setActionLoading(null), 500);
    }
  };

  const handleLogout = async () => {
    await logout();
    setShowLogoutModal(false);
    setAuthenticated(false);
  };

  if (authenticated === null || setupComplete === null) {
    return <div className="min-h-screen bg-background flex items-center justify-center">
      <div className="text-muted-foreground">Loading...</div>
    </div>;
  }

  if (!setupComplete) {
    return <SetupWizard onComplete={() => { setSetupComplete(true); getAuthStatus().then(s => setAuthenticated(s.authenticated)); }} />;
  }

  if (!authenticated) {
    return <LoginPage onLogin={() => setAuthenticated(true)} />;
  }

  return (
    <>
      <Routes>
        <Route path="/setup" element={<SetupWizard reconfigure onComplete={() => { setSetupComplete(true); window.location.assign('/loxone'); }} />} />
        <Route path="/loxone" element={<LoxoneIntegrationPage liveActivities={loxoneActivities} liveStatuses={statuses} liveAvailabilities={availabilities} streamConnected={isConnected} />} />
        <Route path="/system" element={<SystemUpdatesPage returnSlug={devices[0]?.slug} />} />
        <Route path="/devices/:slug/*" element={
          <DeviceLayout
            devices={liveDevices}
            statuses={statuses}
            globalNotAtHome={globalNotAtHome}
            setGlobalNotAtHome={setGlobalNotAtHome}
            isConnected={isConnected}
            error={error}
            reconnect={reconnect}
            theme={theme}
            toggleTheme={toggleTheme}
            onLogout={() => setShowLogoutModal(true)}
          />
        }>
          <Route index element={
            <DeviceHome
              devices={liveDevices}
              statuses={statuses}
              scheduleStates={scheduleStates}
              scenesBySlug={scenesBySlug}
              activeSceneId={activeSceneId}
              actionLoading={actionLoading}
              handleAction={handleAction}
              setPendingScene={setPendingScene}
              setPendingSceneSlug={setPendingSceneSlug}
              setActiveSceneId={setActiveSceneId}
            />
          } />
          <Route path="controls" element={
            <ControlsRoute statuses={statuses} actionLoading={actionLoading} handleAction={handleAction} />
          } />
          <Route path="schedule" element={
            <ScheduleRoute devices={liveDevices} scenesBySlug={scenesBySlug} scheduleStates={scheduleStates} />
          } />
          <Route path="maintenance" element={
            <MaintenanceRoute devices={liveDevices} statuses={statuses} />
          } />
        </Route>
        <Route path="*" element={
          devices.length > 0 ? <Navigate to={`/devices/${devices[0].slug}`} replace /> : (
            <div className="min-h-screen bg-background flex items-center justify-center">
              <div className="text-muted-foreground">Loading devices...</div>
            </div>
          )
        } />
      </Routes>

      <ConfirmModal
        open={showLogoutModal}
        title="Log out"
        message="You are about to log out from Roborock. You will need to re-authenticate with a verification code sent to your email."
        confirmLabel="Log out"
        confirmVariant="destructive"
        onConfirm={handleLogout}
        onCancel={() => setShowLogoutModal(false)}
      />

      <ConfirmModal
        open={pendingScene !== null}
        title={`Start "${pendingScene?.name}"?`}
        message="This will start the cleaning program on the device."
        confirmLabel="Start"
        onConfirm={() => {
          if (pendingScene) {
            setActiveSceneId(pendingScene.id);
            handleAction(`scene-${pendingScene.id}`, () => executeScene(pendingSceneSlug, pendingScene.id));
          }
          setPendingScene(null);
        }}
        onCancel={() => setPendingScene(null)}
      />
    </>
  );
}

// --- Device Layout (shared header + outlet) ---

import { Outlet } from 'react-router-dom';
import type { VacuumStatus } from '@/types/status';
import type { ScheduleState } from '@/types/schedule';

function DeviceLayout({ devices, statuses, globalNotAtHome, setGlobalNotAtHome, isConnected, error, reconnect, theme, toggleTheme, onLogout }: {
  devices: DeviceSummary[];
  statuses: Record<string, VacuumStatus>;
  globalNotAtHome: boolean;
  setGlobalNotAtHome: (v: boolean) => void;
  isConnected: boolean;
  error: string | null;
  reconnect: () => void;
  theme: string;
  toggleTheme: () => void;
  onLogout: () => void;
}) {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();

  return (
    <div className="min-h-screen bg-background p-4 md:p-8">
      <div className="max-w-md mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-2xl font-bold text-foreground">Roborock</h1>
          <div className="flex items-center gap-1">
            <button
              onClick={async () => {
                const newVal = !globalNotAtHome;
                setGlobalNotAtHome(newVal);
                try { await setNotAtHome(newVal); } catch { setGlobalNotAtHome(!newVal); }
              }}
              className="flex items-center gap-1.5 px-2 py-1 rounded-lg hover:bg-accent transition-colors"
              aria-label="Toggle not at home"
              title={globalNotAtHome ? 'Away from home (click to set at home)' : 'At home (click to set away)'}
            >
              <Home className={`h-4 w-4 ${globalNotAtHome ? 'text-gray-500' : 'text-foreground'}`} />
              <div className={`relative w-8 h-[18px] rounded-full transition-colors ${globalNotAtHome ? 'bg-gray-500' : 'bg-green-500'}`}>
                <div className={`absolute top-[2px] h-[14px] w-[14px] rounded-full bg-white transition-transform ${globalNotAtHome ? 'left-[2px]' : 'left-[16px]'}`} />
              </div>
            </button>
            <div className="p-2" title={isConnected ? 'Connected' : 'Disconnected'}>
              {isConnected ? (
                <Wifi className="h-5 w-5 text-green-500" />
              ) : (
                <WifiOff className="h-5 w-5 text-red-500 cursor-pointer" onClick={reconnect} />
              )}
            </div>
            <button onClick={toggleTheme} className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="Toggle theme">
              {theme === 'dark' ? <Sun className="h-5 w-5 text-foreground" /> : <Moon className="h-5 w-5 text-foreground" />}
            </button>
            <Link to="/setup" className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="Integration settings" title="Integration settings">
              <Network className="h-5 w-5 text-foreground" />
            </Link>
            <Link to="/system" className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="System and updates" title="System & Updates">
              <ServerCog className="h-5 w-5 text-foreground" />
            </Link>
            <button onClick={onLogout} className="p-2 rounded-lg hover:bg-accent transition-colors" aria-label="Logout" title="Logout">
              <LogOut className="h-5 w-5 text-foreground" />
            </button>
          </div>
        </div>

        {error && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-500 text-sm">
            {error}
            <button onClick={reconnect} className="ml-2 underline hover:no-underline">Retry</button>
          </div>
        )}

        <DeviceSwitcher devices={devices} statuses={statuses} selected={slug ?? ''} onSelect={(s) => navigate(`/devices/${s}`)} />

        <Outlet />

        <div className="mt-8 text-center text-xs text-muted-foreground">roborock-mqtt-loxone</div>
      </div>
    </div>
  );
}

// --- Device Home (main view) ---

function DeviceHome({ devices, statuses, scheduleStates, scenesBySlug, activeSceneId, actionLoading, handleAction, setPendingScene, setPendingSceneSlug, setActiveSceneId }: {
  devices: DeviceSummary[];
  statuses: Record<string, VacuumStatus>;
  scheduleStates: Record<string, ScheduleState>;
  scenesBySlug: Record<string, SceneInfo[]>;
  activeSceneId: number | null;
  actionLoading: string | null;
  handleAction: (action: string, fn: () => Promise<void>) => void;
  setPendingScene: (s: SceneInfo | null) => void;
  setPendingSceneSlug: (s: string) => void;
  setActiveSceneId: (id: number | null) => void;
}) {
  const { slug } = useParams<{ slug: string }>();
  if (!slug) return null;

  const status = statuses[slug];
  const isEmptyStatus = status ? !status.state : false;
  const isCleaning = status ? activeCleaningStates.has(status.state) : false;
  const scenes = scenesBySlug[slug] ?? [];
  const deviceName = devices.find(d => d.slug === slug)?.name ?? slug;

  // Clear active scene when device stops cleaning
  useEffect(() => {
    if (!isCleaning) setActiveSceneId(null);
  }, [isCleaning]);

  return (
    <>
      {/* Status card */}
      {status && isEmptyStatus ? (
        <div className="mb-6 p-4 bg-card rounded-lg border border-border">
          <p className="text-sm text-muted-foreground">Waiting for status...</p>
        </div>
      ) : status && activeCleaningStates.has(status.state) ? (
        <CleaningProgress status={status} scenes={scenes} />
      ) : status && (
        <div className="mb-6 p-4 bg-card rounded-lg border border-border">
          <div className="flex items-center justify-between mb-3">
            <div className="text-lg font-medium text-foreground">{formatDisplayName(status.state)}</div>
            {status.error_code > 0 && (
              <div className="flex items-center gap-1 text-red-500 text-sm">
                <AlertCircle className="h-4 w-4" />
                <span>{status.error || `Error ${status.error_code}`}</span>
              </div>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div className="flex items-center gap-2">
              <Battery className={`h-4 w-4 ${status.battery > 50 ? 'text-green-500' : status.battery > 20 ? 'text-amber-500' : 'text-red-500'}`} />
              <span className="text-muted-foreground tabular-nums">{status.battery}%</span>
            </div>
            <div className="flex items-center gap-2">
              <Wind className="h-4 w-4 text-foreground" />
              <span className="text-muted-foreground">{formatDisplayName(status.fan_speed)}</span>
            </div>
            <div className="flex items-center gap-2">
              <Droplets className="h-4 w-4 text-foreground" />
              <span className="text-muted-foreground">{formatDisplayName(status.mop_mode)}</span>
            </div>
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-foreground" />
              <span className="text-muted-foreground tabular-nums">{formatCleanTime(status.clean_time)}</span>
            </div>
            {status.clean_area > 0 && (
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 text-foreground" />
                <span className="text-muted-foreground tabular-nums">{formatCleanArea(status.clean_area)}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Inline Pause/Dock during cleaning */}
      {isCleaning && (
        <div className="mb-6 grid grid-cols-2 gap-3">
          <button onClick={() => handleAction('pause', () => pauseCleaning(slug))} disabled={actionLoading === 'pause'} className="p-3 rounded-lg border border-border bg-card hover:bg-accent transition-colors touch-target flex items-center justify-center gap-2">
            <Pause className="h-5 w-5 text-amber-500" /><span className="text-sm text-muted-foreground">Pause</span>
          </button>
          <button onClick={() => handleAction('dock', () => dockVacuum(slug))} disabled={actionLoading === 'dock'} className="p-3 rounded-lg border border-border bg-card hover:bg-accent transition-colors touch-target flex items-center justify-center gap-2">
            <Home className="h-5 w-5 text-primary" /><span className="text-sm text-muted-foreground">Dock</span>
          </button>
        </div>
      )}

      {/* Programs */}
      {scenes.length > 0 && (
        <div className="mb-6">
          <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide mb-3">Programs</h2>
          <div className="space-y-2">
            {scenes.map((scene) => (
              <button
                key={scene.id}
                onClick={() => { setPendingScene(scene); setPendingSceneSlug(slug); }}
                disabled={actionLoading?.startsWith('scene-') ?? false}
                className={`w-full p-3 rounded-lg border-2 transition-all touch-target flex items-center justify-between ${
                  activeSceneId === scene.id && isCleaning ? 'border-primary bg-primary/10' : 'border-border bg-card hover:bg-accent'
                }`}
              >
                <span className="flex flex-col items-start min-w-0">
                  <span className={`text-sm truncate ${activeSceneId === scene.id && isCleaning ? 'text-primary font-medium' : 'text-foreground'}`}>{scene.name}</span>
                  {scene.recorded_minutes != null && (
                    <span className="text-xs text-muted-foreground tabular-nums">~{scene.recorded_minutes} min</span>
                  )}
                </span>
                {activeSceneId === scene.id && isCleaning
                  ? <span className="text-xs uppercase tracking-wide bg-primary text-primary-foreground px-2 py-0.5 rounded">Active</span>
                  : <Play className="h-4 w-4 text-green-500 shrink-0" />
                }
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Controls summary */}
      {status && !isEmptyStatus && (
        <div className="mb-6">
          <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide mb-3">Controls</h2>
          <Link to={`/devices/${slug}/controls`} className="block w-full p-4 bg-card rounded-lg border border-border hover:bg-accent transition-colors text-left">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <div className="flex items-center gap-1.5"><Wind className="h-4 w-4" /><span>{formatDisplayName(status.fan_speed)}</span></div>
                <div className="flex items-center gap-1.5"><Droplets className="h-4 w-4" /><span>{formatDisplayName(status.mop_mode)}</span></div>
              </div>
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            </div>
          </Link>
        </div>
      )}

      {/* Maintenance summary */}
      {status && !isEmptyStatus && status.consumable_percents && (
        <div className="mb-6">
          <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide mb-3">Maintenance</h2>
          <Link
            to={`/devices/${slug}/maintenance`}
            className={`block w-full p-4 bg-card rounded-lg border hover:bg-accent transition-colors text-left ${
              Math.min(status.consumable_percents.main_brush, status.consumable_percents.side_brush, status.consumable_percents.filter, status.consumable_percents.sensor, status.consumable_percents.dust_collection) <= 20
                ? 'border-red-500/30' : 'border-border'
            }`}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3 text-sm">
                <Wrench className="h-4 w-4 text-muted-foreground" />
                {(() => {
                  const p = status.consumable_percents;
                  const worst = Math.min(p.main_brush, p.side_brush, p.filter, p.sensor, p.dust_collection);
                  const worstName = worst === p.main_brush ? 'Main Brush' : worst === p.side_brush ? 'Side Brush' : worst === p.filter ? 'Filter' : worst === p.sensor ? 'Sensor' : 'Dust Collection';
                  if (worst > 50) return <span className="text-green-500">All good</span>;
                  if (worst > 20) return <span className="text-amber-500">{worstName}: {worst}%</span>;
                  return <span className="text-red-500">{worstName}: {worst}%</span>;
                })()}
              </div>
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            </div>
          </Link>
        </div>
      )}

      {/* Schedule */}
      <ScheduleSection
        slug={slug}
        deviceName={deviceName}
        scenes={scenes}
        sseScheduleState={scheduleStates[slug]}
      />

      {/* Loxone integration */}
      <div className="mb-6">
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide mb-3">Integration</h2>
        <Link to="/loxone" className="block w-full p-4 bg-card rounded-lg border border-border hover:bg-accent transition-colors text-left">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3 text-sm"><Network className="h-4 w-4 text-primary" /><span>Loxone Integration</span></div>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
        </Link>
        <Link to="/system" className="mt-2 block w-full p-4 bg-card rounded-lg border border-border hover:bg-accent transition-colors text-left">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3 text-sm"><ServerCog className="h-4 w-4 text-primary" /><span>System & Updates</span></div>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
        </Link>
      </div>

      {/* Map */}
      <VectorMap slug={slug} isCleaning={status?.in_cleaning ?? false} />
    </>
  );
}

// --- Route wrappers for sub-pages ---

function ControlsRoute({ statuses, actionLoading, handleAction }: {
  statuses: Record<string, VacuumStatus>;
  actionLoading: string | null;
  handleAction: (action: string, fn: () => Promise<void>) => void;
}) {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  if (!slug) return null;
  return <ControlsPage slug={slug} status={statuses[slug]} actionLoading={actionLoading} onAction={handleAction} onClose={() => navigate(`/devices/${slug}`)} />;
}

function ScheduleRoute({ devices, scenesBySlug, scheduleStates }: {
  devices: DeviceSummary[];
  scenesBySlug: Record<string, SceneInfo[]>;
  scheduleStates: Record<string, ScheduleState>;
}) {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  if (!slug) return null;
  const deviceName = devices.find(d => d.slug === slug)?.name ?? slug;
  return <SchedulePage slug={slug} deviceName={deviceName} scenes={scenesBySlug[slug] ?? []} sseScheduleState={scheduleStates[slug]} onClose={() => navigate(`/devices/${slug}`)} />;
}

function MaintenanceRoute({ devices, statuses }: {
  devices: DeviceSummary[];
  statuses: Record<string, VacuumStatus>;
}) {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  if (!slug) return null;
  const status = statuses[slug];
  if (!status) return null;
  const deviceName = devices.find(d => d.slug === slug)?.name ?? slug;
  return <MaintenancePage slug={slug} deviceName={deviceName} percents={status.consumable_percents} consumables={status.consumables} onClose={() => navigate(`/devices/${slug}`)} />;
}
