import { useI18n } from 'vue-i18n';
import { useSimBarStore, type SimBarSnapshot } from '../stores/simBar';
import { usePreferencesStore } from '../stores/preferences';
import { useToastStore } from '../stores/toast';
import type { LayoutSnapshot } from '../stores/preferences';

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
