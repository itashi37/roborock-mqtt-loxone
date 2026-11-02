import type { VacuumStatus } from '@/types/status';

const states: Record<string, string> = {
  charging: 'En charge', docked: 'À la station', idle: 'Prêt à nettoyer', sleeping: 'En veille',
  cleaning: 'Nettoyage en cours', segment_cleaning: 'Nettoyage des pièces', zoned_cleaning: 'Nettoyage de zone', spot_cleaning: 'Nettoyage localisé',
  returning_home: 'Retour à la station', going_to_target: 'En déplacement', paused: 'En pause', error: 'À vérifier',
  washing_mop: 'Lavage des serpillières', going_to_wash_mop: 'Retour pour lavage', drying_mop: 'Séchage des serpillières', emptying_dustbin: 'Vidage du bac',
  cleaning_started: 'Nettoyage démarré', cleaning_finished: 'Nettoyage terminé', cleaning_completed: 'Nettoyage terminé', room_entered: 'Changement de pièce', docked_event: 'Retour à la station',
};
export function robotStateLabel(state?: string): string { return state ? states[state] ?? state.replace(/_/g, ' ') : 'En attente d’un état'; }
export function robotNeedsAttention(status?: VacuumStatus | null): boolean { return Boolean(status && (status.error_code || status.dock_error_status)); }
