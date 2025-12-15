import React, { useCallback, useEffect, useMemo, useState } from "https://esm.sh/react@18.2.0";
import { createRoot } from "https://esm.sh/react-dom@18.2.0/client";

type DocumentRecord = {
  id: string;
  name?: string;
  fileName?: string;
  status?: string;
  updatedAt?: string;
  content?: string;
};

type ApiStatus = "connecting" | "online" | "offline";

const STORAGE_KEY = "vaultdrop:documents";

const App = () => {
  const apiBase = useMemo(() => window.VAULTDROP_API ?? inferApiBase(), []);
  const [apiStatus, setApiStatus] = useState<ApiStatus>("connecting");
  const [docIds, setDocIds] = useState<string[]>([]);
  const [documents, setDocuments] = useState<DocumentRecord[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [previewText, setPreviewText] = useState("Select a document to view the extracted text.");
  const [isUploading, setUploading] = useState(false);
  const [toast, setToast] = useState<string | null>(null);

  useEffect(() => {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      try {
        const ids = JSON.parse(raw) as string[];
        if (Array.isArray(ids) && ids.length > 0) {
          setDocIds(ids);
        }
      } catch (err) {
        console.warn("failed to parse storage", err);
      }
    }
  }, []);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(docIds));
  }, [docIds]);

  const checkHealth = useCallback(async () => {
    try {
      const res = await fetch(`${apiBase}/healthz`);
      setApiStatus(res.ok ? "online" : "offline");
    } catch {
      setApiStatus("offline");
    }
  }, [apiBase]);

  useEffect(() => {
    void checkHealth();
  }, [checkHealth]);

  const refreshDocuments = useCallback(async () => {
    if (docIds.length === 0) {
      setDocuments([]);
      return;
    }
    const next = await Promise.all(
      docIds.map(async (id: string) => {
        try {
          const res = await fetch(`${apiBase}/documents/${id}`);
          if (!res.ok) throw new Error("failed");
          return (await res.json()) as DocumentRecord;
        } catch (err) {
          console.warn("fetch document failed", id, err);
          return { id, status: "unknown" };
        }
      })
    );
    setDocuments(next);
  }, [apiBase, docIds]);

  useEffect(() => {
    void refreshDocuments();
  }, [refreshDocuments]);

  const loadPreview = useCallback(
    async (id: string) => {
      setPreviewText("Loading text…");
      try {
        const res = await fetch(`${apiBase}/documents/${id}/text`);
        if (res.status === 202) {
          setPreviewText("Document is still processing. Check back shortly.");
          return;
        }
        if (!res.ok) {
          const message = await res.text();
          setPreviewText(message || "Failed to load text.");
          return;
        }
        const text = await res.text();
        setPreviewText(text || "(No text extracted)");
      } catch (err) {
        console.error(err);
        setPreviewText("Unable to load preview.");
      }
    },
    [apiBase]
  );


  useEffect(() => {
    const pending = documents.some((doc) => doc.status && doc.status !== "completed");
    if (!pending) {
      return;
    }
    const timer = window.setInterval(() => {
      void (async () => {
        await refreshDocuments();
        if (selectedId) {
          await loadPreview(selectedId);
        }
      })();
    }, 4000);
    return () => window.clearInterval(timer);
  }, [documents, refreshDocuments, selectedId, loadPreview]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(null), 2500);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const handleUpload = async (event: SubmitEvent) => {
    event.preventDefault();
    const formElement = event.currentTarget as HTMLFormElement | null;
    const input = formElement?.elements.namedItem("file") as HTMLInputElement | null;
    const file = input?.files?.[0];
    if (!file) return;

    const form = new FormData();
    form.append("file", file);
    setUploading(true);
    try {
      const res = await fetch(`${apiBase}/documents`, { method: "POST", body: form });
      if (!res.ok) {
        const message = await res.text();
        throw new Error(message || "Upload failed");
      }
      const body = (await res.json()) as { id: string; status: string };
      setDocIds((prev: string[]) => [body.id, ...prev.filter((id: string) => id !== body.id)]);
      setToast("Upload queued");
      if (input) input.value = "";
      setSelectedId(body.id);
      await refreshDocuments();
      await loadPreview(body.id);
    } catch (err) {
      console.error(err);
      setToast("Upload failed");
    } finally {
      setUploading(false);
    }
  };

  const downloadProcessed = useCallback(
    async (id: string) => {
      try {
        const res = await fetch(`${apiBase}/documents/${id}/processed-url`);
        if (!res.ok) throw new Error("Failed to get url");
        const body = (await res.json()) as { url: string };
        window.open(body.url, "_blank");
      } catch (err) {
        console.error(err);
        setToast("Download link unavailable");
      }
    },
    [apiBase]
  );

  const handleSelectDocument = async (id: string) => {
    setSelectedId(id);
    await loadPreview(id);
  };

  const selectedDoc = documents.find((doc) => doc.id === selectedId) ?? null;

  return (
    <>
      <header>
        <div className="brand">VaultDrop</div>
        <div className="status" style={{ color: statusColor(apiStatus) }}>
          {apiStatus === "connecting" ? "Connecting..." : apiStatus === "online" ? "API online" : "API offline"}
        </div>
      </header>
      <main>
        <section className="card">
          <h2>Upload a PDF</h2>
          <form id="upload-form" onSubmit={handleUpload}>
            <input type="file" name="file" accept="application/pdf" required />
            <button type="submit" disabled={isUploading}>
              {isUploading ? "Uploading…" : "Upload"}
            </button>
          </form>
          <p className="hint">Upload a PDF to extract its text. Files are streamed directly to the VaultDrop API.</p>
        </section>

        <section className="card">
          <div className="section-header">
            <h2>Recent Documents</h2>
            <button className="ghost" onClick={() => void refreshDocuments()}>
              Refresh
            </button>
          </div>
          <div className={`document-list ${documents.length === 0 ? "empty-state" : ""}`} id="documents-list">
            {documents.length === 0 ? (
              <p>No documents yet. Upload a PDF to begin.</p>
            ) : (
              documents.map((doc: DocumentRecord) => (
                <div
                  key={doc.id}
                  className={`document-row ${doc.id === selectedId ? "active" : ""}`}
                  onClick={() => void handleSelectDocument(doc.id)}
                >
                  <div>
                    <div className="doc-name">{doc.name ?? doc.fileName ?? doc.id}</div>
                    <div className="doc-meta">
                      ID: {doc.id} · Updated {formatRelative(doc.updatedAt)}
                    </div>
                  </div>
                  <span className="doc-status" style={{ color: statusColor(doc.status) }}>
                    {doc.status ?? "unknown"}
                  </span>
                </div>
              ))
            )}
          </div>
        </section>

        <section className="card">
          <div className="section-header">
            <h2>Document Preview</h2>
            <button
              className="ghost"
              disabled={!selectedDoc || selectedDoc.status !== "completed"}
              onClick={() => selectedDoc && void downloadProcessed(selectedDoc.id)}
            >
              Download Text
            </button>
          </div>
          <div className="preview">{previewText}</div>
        </section>
      </main>
      {toast && (
        <div className="toast show" role="status">
          {toast}
        </div>
      )}
    </>
  );
};

function inferApiBase(): string {
  const location = window.location;
  if (location.port && location.port !== "4173") {
    return `${location.protocol}//${location.hostname}:${location.port}`;
  }
  return `${location.protocol}//${location.hostname}:8080`;
}

function formatRelative(ts?: string): string {
  if (!ts) return "unknown";
  const date = new Date(ts);
  const diff = Date.now() - date.getTime();
  if (diff < 60 * 1000) return "just now";
  if (diff < 60 * 60 * 1000) return `${Math.floor(diff / (60 * 1000))}m ago`;
  if (diff < 24 * 60 * 60 * 1000) return `${Math.floor(diff / (60 * 60 * 1000))}h ago`;
  return date.toLocaleDateString();
}

function statusColor(status?: string | ApiStatus): string {
  switch ((status ?? "").toString().toLowerCase()) {
    case "online":
    case "completed":
      return "#22c55e";
    case "processing":
    case "queued":
    case "connecting":
      return "#a78bfa";
    case "offline":
    case "failed":
    case "rejected":
      return "#fb7185";
    default:
      return "#e2e8f0";
  }
}

declare global {
  interface Window {
    VAULTDROP_API?: string;
  }
}

const container = document.getElementById("root");
if (!container) {
  throw new Error("Missing #root element");
}
const root = createRoot(container);
root.render(<App />);
