<template>
  <div class="card">
    <div class="icon">{{ pluginIcon }}</div>
    <h3 class="cardTitle">{{ plugin.title }}</h3>
    <p class="cardDescription">{{ plugin.description }}</p>
    <div class="installBox">
      <div class="installRow">
        <code class="installCommand">{{ cmd }}</code>
        <CopyButton :text="cmd" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from "vue";
import CopyButton from "./CopyButton.vue";

const props = defineProps({
  plugin: {
    type: Object,
    required: true,
  },
});

const RAW_BASE =
  "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/";

const pluginUrl = computed(() => props.plugin.url || `${RAW_BASE}${props.plugin.file}`);
const cmd = computed(() => `matcha install ${pluginUrl.value}`);
const pluginIcon = computed(() => props.plugin.title.charAt(0).toUpperCase());
</script>

<style scoped>
.card {
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 1.5rem;
  background: var(--vp-c-bg);
  transition: transform 0.2s, border-color 0.2s, box-shadow 0.2s;
  display: flex;
  flex-direction: column;
}

.card:hover {
  border-color: var(--accent);
  box-shadow: 0 8px 30px rgba(74, 222, 128, 0.12);
  transform: translateY(-2px);
}

.icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(74, 222, 128, 0.2), rgba(34, 197, 94, 0.15));
  color: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 1.1rem;
  margin-bottom: 1rem;
}

.cardTitle {
  font-size: 1.15rem;
  font-weight: 600;
  color: var(--text);
  margin: 0 0 0.5rem 0;
}

.cardDescription {
  color: var(--muted);
  font-size: 0.9rem;
  margin-bottom: 1.25rem;
  line-height: 1.5;
  flex: 1;
}

.installBox {
  background: var(--code-bg);
  border-radius: 10px;
  padding: 0.75rem;
}

.installRow {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.installCommand {
  background: transparent;
  border-radius: 6px;
  padding: 0.25rem 0;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  overflow-x: auto;
  white-space: nowrap;
  color: var(--text);
  flex: 1;
  min-width: 0;
}
</style>
