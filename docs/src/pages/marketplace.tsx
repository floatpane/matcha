import React, { useState } from "react";
import Layout from "@theme/Layout";
import styles from "./marketplace.module.css";
import pluginsData from "@site/static/plugins.json";

interface Plugin {
  name: string;
  title: string;
  description: string;
  file: string;
  url?: string;
}

const plugins: Plugin[] = pluginsData;
const RAW_BASE =
  "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/";

function pluginUrl(plugin: Plugin): string {
  return plugin.url || `${RAW_BASE}${plugin.file}`;
}

function installCmd(plugin: Plugin): string {
  return `matcha install ${pluginUrl(plugin)}`;
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      className={styles.copyButton}
      onClick={handleCopy}
      title="Copy to clipboard"
      type="button"
    >
      {copied ? "Copied!" : "Copy"}
    </button>
  );
}

function PluginCard({ plugin }: { plugin: Plugin }) {
  const cmd = installCmd(plugin);

  return (
    <div className={styles.card}>
      <h3 className={styles.cardTitle}>{plugin.title}</h3>
      <p className={styles.cardDescription}>{plugin.description}</p>
      <div className={styles.installLabel}>Install:</div>
      <div className={styles.installRow}>
        <code className={styles.installCommand}>{cmd}</code>
        <CopyButton text={cmd} />
      </div>
    </div>
  );
}

export default function Marketplace(): React.JSX.Element {
  return (
    <Layout
      title="Plugin Marketplace"
      description="Browse and install Matcha plugins"
    >
      <div className={styles.marketplace}>
        <div className={styles.header}>
          <h1>Plugin Marketplace</h1>
          <p>
            Browse community plugins for Matcha. Click install commands to copy.
          </p>
        </div>
        <p className={styles.count}>{plugins.length} plugins available</p>
        <div className={styles.grid}>
          {plugins.map((plugin) => (
            <PluginCard key={plugin.name} plugin={plugin} />
          ))}
        </div>
      </div>
    </Layout>
  );
}
