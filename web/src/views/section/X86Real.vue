<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import SectionPlaceholder from './SectionPlaceholder.vue';

const { t } = useI18n();

const preview = computed(() => [
  t('sections.x86.preview1'),
  t('sections.x86.preview2'),
  t('sections.x86.preview3'),
  t('sections.x86.preview4'),
  t('sections.x86.preview5'),
]);
</script>

<template>
  <SectionPlaceholder
    :title="t('sections.x86.title')"
    :tagline="t('sections.x86.tagline')"
    status="soon"
    :preview="preview"
  />
</template>
