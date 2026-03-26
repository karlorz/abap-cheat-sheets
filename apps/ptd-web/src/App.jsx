import { startTransition, useDeferredValue, useEffect, useState } from "react";

const EXAMPLE_FILTERS = {
  "fi-origin-monthly": {
    posting_from: "202602",
    posting_to: "202603",
    company_code: "1000",
    fi_origin: "MKPF"
  },
  "ar-open-monthly": {
    posting_from: "202601",
    company_code: "1000",
    currency_code: "HKD",
    document_type: "DR"
  },
  "ar-cleared-monthly": {
    clearing_from: "202601",
    clearing_to: "202603",
    company_code: "1000",
    original_document_type: "RV",
    clearing_document_type: "DZ"
  },
  "purchasing-history-monthly": {
    posting_from: "202601",
    posting_to: "202603",
    history_category: "2",
    plant_code: "6110"
  },
  "inventory-movement-monthly": {
    posting_from: "202601",
    posting_to: "202603",
    plant_code: "6110",
    movement_type: "101"
  },
  "current-stock": {
    plant_code: "6110",
    storage_location: "3G"
  }
};

const API_NOTE = "API proxy target: /api via Vite dev server";

function buildDatasetUrl(datasetId, params) {
  const query = new URLSearchParams();

  Object.entries(params).forEach(([key, value]) => {
    if (value !== "" && value != null) {
      query.set(key, value);
    }
  });

  const search = query.toString();
  return `/api/datasets/${datasetId}/rows${search ? `?${search}` : ""}`;
}

async function readJSON(path) {
  const response = await fetch(path);
  const data = await response.json();

  if (!response.ok) {
    const message = data?.message || `Request failed with status ${response.status}`;
    const error = new Error(message);
    error.status = response.status;
    error.payload = data;
    throw error;
  }

  return data;
}

function formatPayload(value) {
  return JSON.stringify(value, null, 2);
}

function emptyPreviewState() {
  return {
    mode: "idle",
    requestPath: "",
    payload: null,
    error: null
  };
}

export default function App() {
  const [catalogState, setCatalogState] = useState({
    loading: true,
    error: null,
    items: []
  });
  const [selectedId, setSelectedId] = useState("");
  const [datasetState, setDatasetState] = useState({
    loading: false,
    error: null,
    dataset: null
  });
  const [formState, setFormState] = useState({
    limit: "100",
    filters: {}
  });
  const [resultState, setResultState] = useState(emptyPreviewState());
  const deferredResultPayload = useDeferredValue(resultState.payload);
  const [formattedPayload, setFormattedPayload] = useState("");

  useEffect(() => {
    let ignore = false;

    async function loadCatalog() {
      try {
        const data = await readJSON("/api/datasets");
        if (ignore) {
          return;
        }

        const items = data.items || [];
        setCatalogState({
          loading: false,
          error: null,
          items
        });

        if (items.length > 0) {
          startTransition(() => {
            setSelectedId(items[0].id);
          });
        }
      } catch (error) {
        if (ignore) {
          return;
        }

        setCatalogState({
          loading: false,
          error: error.message,
          items: []
        });
      }
    }

    loadCatalog();
    return () => {
      ignore = true;
    };
  }, []);

  useEffect(() => {
    if (!selectedId) {
      return;
    }

    let ignore = false;
    setDatasetState((current) => ({
      ...current,
      loading: true,
      error: null
    }));

    async function loadDataset() {
      try {
        const data = await readJSON(`/api/datasets/${selectedId}`);
        if (ignore) {
          return;
        }

        const dataset = data.dataset;
        const defaultLimit = String(dataset?.limit?.default ?? 100);
        const exampleFilters = EXAMPLE_FILTERS[selectedId] || {};

        startTransition(() => {
          setDatasetState({
            loading: false,
            error: null,
            dataset
          });
          setFormState({
            limit: defaultLimit,
            filters: { ...exampleFilters }
          });
          setResultState(emptyPreviewState());
        });
      } catch (error) {
        if (ignore) {
          return;
        }

        setDatasetState({
          loading: false,
          error: error.message,
          dataset: null
        });
      }
    }

    loadDataset();
    return () => {
      ignore = true;
    };
  }, [selectedId]);

  useEffect(() => {
    startTransition(() => {
      setFormattedPayload(deferredResultPayload ? formatPayload(deferredResultPayload) : "");
    });
  }, [deferredResultPayload]);

  const selectedDataset = datasetState.dataset;
  const supportedFilters = selectedDataset?.planned_filters || [];
  const executableFilters = catalogState.items.find((item) => item.id === selectedId)?.executable_filters || [];

  async function submit(mode) {
    if (!selectedId) {
      return;
    }

    const params = {
      limit: formState.limit || String(selectedDataset?.limit?.default ?? 100)
    };

    if (mode === "dry_run") {
      params.dry_run = "true";
    }

    executableFilters.forEach((filterName) => {
      const value = formState.filters[filterName];
      if (value) {
        params[filterName] = value;
      }
    });

    const requestPath = buildDatasetUrl(selectedId, params);

    setResultState({
      mode,
      requestPath,
      payload: null,
      error: null
    });

    try {
      const payload = await readJSON(requestPath);
      startTransition(() => {
        setResultState({
          mode,
          requestPath,
          payload,
          error: null
        });
      });
    } catch (error) {
      startTransition(() => {
        setResultState({
          mode,
          requestPath,
          payload: error.payload || null,
          error: error.message
        });
      });
    }
  }

  return (
    <div className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">PTD Dashboard Stack</p>
          <h1>Dataset Explorer</h1>
          <p className="hero-copy">
            A thin frontend over the contract-backed API. Inspect dataset metadata,
            try safe filter combinations, and preview the exact SQL the backend will run.
          </p>
        </div>
        <div className="hero-card">
          <span className="hero-label">Current mode</span>
          <strong>Read-only analytics only</strong>
          <p>{API_NOTE}</p>
        </div>
      </header>

      <main className="layout">
        <aside className="catalog-panel">
          <div className="panel-heading">
            <h2>Datasets</h2>
            <span>{catalogState.items.length}</span>
          </div>
          {catalogState.loading ? <p className="status-card">Loading dataset catalog...</p> : null}
          {catalogState.error ? <p className="status-card error">{catalogState.error}</p> : null}
          <div className="dataset-list">
            {catalogState.items.map((item) => (
              <button
                key={item.id}
                type="button"
                className={`dataset-card ${item.id === selectedId ? "active" : ""}`}
                onClick={() => {
                  startTransition(() => {
                    setSelectedId(item.id);
                  });
                }}
              >
                <div className="dataset-topline">
                  <span className="domain-chip">{item.domain}</span>
                  <span className="support-chip">{item.filter_support.replaceAll("_", " ")}</span>
                </div>
                <strong>{item.title}</strong>
                <p>{item.id}</p>
                <div className="dataset-meta">
                  <span>{item.columns.length} columns</span>
                  <span>{item.executable_filters.length} executable filters</span>
                </div>
              </button>
            ))}
          </div>
        </aside>

        <section className="workspace">
          <section className="panel dataset-panel">
            <div className="panel-heading">
              <h2>Dataset Detail</h2>
              {selectedDataset ? <span>{selectedDataset.id}</span> : null}
            </div>

            {datasetState.loading ? <p className="status-card">Loading dataset detail...</p> : null}
            {datasetState.error ? <p className="status-card error">{datasetState.error}</p> : null}

            {selectedDataset ? (
              <div className="detail-grid">
                <article className="detail-card">
                  <h3>{selectedDataset.title}</h3>
                  <p className="muted">SQL file: {selectedDataset.sql_file}</p>
                  <div className="tag-row">
                    <span className="tag">domain {selectedDataset.domain}</span>
                    <span className="tag">limit {selectedDataset.limit.default}</span>
                    <span className="tag">cache {selectedDataset.cache_ttl_seconds}s</span>
                  </div>
                </article>

                <article className="detail-card">
                  <h3>Columns</h3>
                  <div className="token-grid">
                    {selectedDataset.columns.map((column) => (
                      <span className="token" key={column}>
                        {column}
                      </span>
                    ))}
                  </div>
                </article>

                <article className="detail-card">
                  <h3>Filter Coverage</h3>
                  <p className="muted">
                    Executable filters are wired in the API today. Planned filters are the
                    contract target shape.
                  </p>
                  <div className="token-grid">
                    {supportedFilters.map((filterName) => (
                      <span
                        className={`token ${
                          executableFilters.includes(filterName) ? "token-live" : "token-planned"
                        }`}
                        key={filterName}
                      >
                        {filterName}
                      </span>
                    ))}
                  </div>
                </article>
              </div>
            ) : null}
          </section>

          <section className="panel action-panel">
            <div className="panel-heading">
              <h2>Query Controls</h2>
              <span>Dry run first</span>
            </div>

            {selectedDataset ? (
              <div className="form-grid">
                <label className="field">
                  <span>limit</span>
                  <input
                    value={formState.limit}
                    onChange={(event) =>
                      setFormState((current) => ({
                        ...current,
                        limit: event.target.value
                      }))
                    }
                    inputMode="numeric"
                  />
                </label>

                {supportedFilters.map((filterName) => (
                  <label className="field" key={filterName}>
                    <span>
                      {filterName}
                      {executableFilters.includes(filterName) ? (
                        <em className="field-hint">live</em>
                      ) : (
                        <em className="field-hint muted">planned</em>
                      )}
                    </span>
                    <input
                      value={formState.filters[filterName] || ""}
                      onChange={(event) =>
                        setFormState((current) => ({
                          ...current,
                          filters: {
                            ...current.filters,
                            [filterName]: event.target.value
                          }
                        }))
                      }
                      disabled={!executableFilters.includes(filterName)}
                      placeholder={EXAMPLE_FILTERS[selectedId]?.[filterName] || ""}
                    />
                  </label>
                ))}
              </div>
            ) : null}

            <div className="action-row">
              <button type="button" className="primary-button" onClick={() => submit("dry_run")}>
                Preview SQL
              </button>
              <button type="button" className="secondary-button" onClick={() => submit("execute")}>
                Run API
              </button>
            </div>

            <p className="muted">
              `Run API` will return rows only when the Go backend has a working `PTD_SQLSERVER_DSN`.
            </p>
          </section>

          <section className="panel result-panel">
            <div className="panel-heading">
              <h2>Response</h2>
              <span>{resultState.mode}</span>
            </div>

            {resultState.requestPath ? (
              <div className="request-path">
                <span>Request</span>
                <code>{resultState.requestPath}</code>
              </div>
            ) : null}

            {resultState.error ? <p className="status-card error">{resultState.error}</p> : null}

            {!resultState.payload && !resultState.error ? (
              <p className="status-card">
                Select a dataset, adjust the live filters, and preview the generated query.
              </p>
            ) : null}

            {resultState.payload ? (
              <pre className="payload-view">
                <code>{formattedPayload}</code>
              </pre>
            ) : null}
          </section>
        </section>
      </main>
    </div>
  );
}
