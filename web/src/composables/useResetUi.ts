import { useI18n } from 'vue-i18n';
import { useSimBarStore, type SimBarSnapshot } from '../stores/simBar';
import { usePreferencesStore } from '../stores/preferences';
import { useToastStore } from '../stores/toast';
import type { LayoutSnapshot } from '../stores/preferences';

// Event name picked up by TutorLauncher to reset the FAB's local drag offset.
// The offset lives in the component (it is intentionally ephemeral and not
// persisted), so a CustomEvent is the lightest coupling.
export const RESET_FAB_EVENT = 'modos:reset-fab-position';

interface ResetSnapshot {
  simBar: SimBarSnapshot;
  layout: LayoutSnapshot;
}

export function useResetUi() {
  const bar = useSimBarStore();
  const prefs = usePreferencesStore();
  const toast = useToastStore();
  const { t } = useI18n();

  return function resetUi() {
    const snapshot: ResetSnapshot = {
      simBar: bar.getState(),
      layout: prefs.getLayoutSnapshot(),
    };

    bar.reset();
    prefs.resetLayout();
    window.dispatchEvent(new CustomEvent(RESET_FAB_EVENT));

    toast.pushAction(
      t('simbar.toasts.resetUiDone'),
      {
        label: t('simbar.toasts.undo'),
        run: () => {
          bar.applyState(snapshot.simBar);
          prefs.applyLayoutSnapshot(snapshot.layout);
        },
      },
      'info',
    );
  };
}
