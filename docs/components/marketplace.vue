<template>
  <div class="marketplace">
    <div v-if="loading" class="loading">Loading plugins...</div>
    <div v-if="error" class="error">Error: {{ error }}</div>
    <div v-if="!loading && !error" class="grid">
      <PluginCard v-for="plugin in plugins" :key="plugin.name" :plugin="plugin" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from "vue";
import PluginCard from "./PluginCard.vue";

const REGISTRY_URL =
  "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/registry.json";

const plugins = ref([]);
const loading = ref(true);
const error = ref(null);

onMounted(() => {
  fetch(REGISTRY_URL)
    .then((res) => {
      if (!res.ok) throw new Error(`Failed to fetch registry (${res.status})`);
      return res.json();
    })
    .then((data) => {
      plugins.value = data;
      loading.value = false;
    })
    .catch((err) => {
      error.value = err.message;
      loading.value = false;
    });
});
</script>

<style scoped>
.marketplace {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 1rem 3rem;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 1.25rem;
}

.loading,
.error {
  text-align: center;
  color: var(--muted);
  padding: 3rem 0;
  font-size: 1.1rem;
}

.error {
  color: var(--danger);
}
</style>
