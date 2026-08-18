import { useEffect, useRef, useState, useCallback } from 'react';
import type { SSEEvent, VacuumStatus } from '@/types/status';
import type { ScheduleState } from '@/types/schedule';
import type { LoxoneActivity } from '@/types/loxone';
import { API_BASE } from '@/lib/api';

interface SSEHookReturn {
  statuses: Record<string, VacuumStatus>;
  scheduleStates: Record<string, ScheduleState>;
  availabilities: Record<string, boolean>;
  loxoneActivities: Record<string, LoxoneActivity>;
  isConnected: boolean;
  error: string | null;
  reconnect: () => void;
}

export function useSSE(): SSEHookReturn {
  const [statuses, setStatuses] = useState<Record<string, VacuumStatus>>({});
  const [scheduleStates, setScheduleStates] = useState<Record<string, ScheduleState>>({});
  const [availabilities, setAvailabilities] = useState<Record<string, boolean>>({});
  const [loxoneActivities, setLoxoneActivities] = useState<Record<string, LoxoneActivity>>({});
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cleanup = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    cleanup();

    try {
      const eventSource = new EventSource(`${API_BASE}/events`);
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        setIsConnected(true);
        setError(null);
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'schedule') {
            setScheduleStates(prev => ({ ...prev, [data.device]: data.state }));
          } else if (data.type === 'availability') {
            setAvailabilities(prev => ({ ...prev, [data.device]: data.online }));
          } else if (data.type === 'loxone_activity') {
            setLoxoneActivities(prev => ({ ...prev, [data.device]: data.activity as LoxoneActivity }));
          } else {
            const { device, ...status } = data as SSEEvent;
            setStatuses(prev => ({ ...prev, [device]: status as VacuumStatus }));
          }
        } catch {
          setError('Failed to parse server data');
        }
      };

      eventSource.onerror = () => {
        setIsConnected(false);
        setError(eventSource.readyState === EventSource.CLOSED
          ? 'Connection closed by server'
          : 'Connection error');

        reconnectTimeoutRef.current = setTimeout(() => {
          if (eventSourceRef.current === eventSource) {
            connect();
          }
        }, 3000);
      };
    } catch {
      setError('Failed to connect to server');
    }
  }, [cleanup]);

  useEffect(() => {
    connect();
    return cleanup;
  }, [connect, cleanup]);

  const reconnect = useCallback(() => {
    setError(null);
    connect();
  }, [connect]);

  return { statuses, scheduleStates, availabilities, loxoneActivities, isConnected, error, reconnect };
}
