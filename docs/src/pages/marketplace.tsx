import React, { useState, useEffect } from "react";
import Layout from "@theme/Layout";
import styles from "./marketplace.module.css";

interface Plugin {
  name: string;
  title: string;
  description: string;
  file: string;
  url?: string;
}

const REGISTRY_URL =
  "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/registry.json";
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
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch(REGISTRY_URL)
      .then((res) => {
        if (!res.ok)
          throw new Error(`Failed to fetch registry (${res.status})`);
        return res.json();
      })
      .then((data: Plugin[]) => {
        setPlugins(data);
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  }, []);

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
        {loading && <p className={styles.count}>Loading plugins...</p>}
        {error && <p className={styles.error}>Error: {error}</p>}
        {!loading && !error && (
          <>
            <p className={styles.count}>{plugins.length} plugins available</p>
            <div className={styles.grid}>
              {plugins.map((plugin) => (
                <PluginCard key={plugin.name} plugin={plugin} />
              ))}
            </div>
          </>
        )}
      </div>
    </Layout>
  );
}
