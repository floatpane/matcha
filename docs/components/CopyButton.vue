<template>
  <button
    class="copyButton"
    @click="handleCopy"
    title="Copy to clipboard"
    type="button"
  >
    {{ copied ? 'Copied!' : 'Copy' }}
  </button>
</template>

<script setup>
import { ref } from "vue";

const props = defineProps({
  text: {
    type: String,
    required: true,
  },
});

const copied = ref(false);

const handleCopy = () => {
  navigator.clipboard.writeText(props.text);
  copied.value = true;
  setTimeout(() => (copied.value = false), 2000);
};
</script>

<style scoped>
.copyButton {
  background: transparent;
  color: var(--muted);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 0.4rem 0.75rem;
  font-size: 0.7rem;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition: color 0.2s, border-color 0.2s, background 0.2s;
}

.copyButton:hover {
  color: var(--accent);
  border-color: var(--accent);
  background: var(--code-bg);
}
</style>
