import { Fragment, useEffect, useState } from "react";
import { adminFetch } from "./api";

export default function ClientsPanel({ token }) {
  const [clients, setClients] = useState([]);
  const [error, setError] = useState("");
  const [newClient, setNewClient] = useState({ name: "", rate_limit_per_minute: 60 });
  const [editingId, setEditingId] = useState(null);
  const [editClient, setEditClient] = useState({ name: "", rate_limit_per_minute: 60 });
  const [expandedId, setExpandedId] = useState(null);
  const [keysByClient, setKeysByClient] = useState({});
  const [revealedKey, setRevealedKey] = useState(null); // { clientId, key }

  async function load() {
    try {
      setClients(await adminFetch(token, "/admin/clients/"));
      setError("");
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    load();
  }, [token]);

  async function loadKeys(clientId) {
    const keys = await adminFetch(token, `/admin/clients/${clientId}/keys/`);
    setKeysByClient((prev) => ({ ...prev, [clientId]: keys }));
  }

  async function handleCreate(e) {
    e.preventDefault();
    try {
      await adminFetch(token, "/admin/clients/", {
        method: "POST",
        body: JSON.stringify({
          name: newClient.name,
          rate_limit_per_minute: Number(newClient.rate_limit_per_minute),
        }),
      });
      setNewClient({ name: "", rate_limit_per_minute: 60 });
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  function startEdit(client) {
    setEditingId(client.id);
    setEditClient({ name: client.name, rate_limit_per_minute: client.rate_limit_per_minute });
  }

  async function handleSaveEdit(id) {
    try {
      await adminFetch(token, `/admin/clients/${id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: editClient.name,
          rate_limit_per_minute: Number(editClient.rate_limit_per_minute),
        }),
      });
      setEditingId(null);
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleDelete(id) {
    if (!confirm("Delete this client? This fails if they still have any API keys on record."))
      return;
    try {
      await adminFetch(token, `/admin/clients/${id}`, { method: "DELETE" });
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  async function toggleKeys(clientId) {
    setRevealedKey(null);
    if (expandedId === clientId) {
      setExpandedId(null);
      return;
    }
    setExpandedId(clientId);
    try {
      await loadKeys(clientId);
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleGenerateKey(clientId) {
    try {
      const rec = await adminFetch(token, `/admin/clients/${clientId}/keys/`, {
        method: "POST",
      });
      setRevealedKey({ clientId, key: rec.key });
      await loadKeys(clientId);
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleRevokeKey(clientId, keyId) {
    if (!confirm("Revoke this key? This can't be undone.")) return;
    try {
      await adminFetch(token, `/admin/clients/${clientId}/keys/${keyId}`, {
        method: "DELETE",
      });
      await loadKeys(clientId);
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <section>
      <h2>Clients</h2>
      {error && <p className="error">{error}</p>}
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Rate Limit / min</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {clients.map((c) => (
            <Fragment key={c.id}>
              {editingId === c.id ? (
                <tr>
                  <td>
                    <input
                      value={editClient.name}
                      onChange={(e) =>
                        setEditClient({ ...editClient, name: e.target.value })
                      }
                    />
                  </td>
                  <td>
                    <input
                      type="number"
                      value={editClient.rate_limit_per_minute}
                      onChange={(e) =>
                        setEditClient({
                          ...editClient,
                          rate_limit_per_minute: e.target.value,
                        })
                      }
                    />
                  </td>
                  <td className="actions">
                    <button onClick={() => handleSaveEdit(c.id)}>Save</button>
                    <button onClick={() => setEditingId(null)}>Cancel</button>
                  </td>
                </tr>
              ) : (
                <tr>
                  <td>{c.name}</td>
                  <td>{c.rate_limit_per_minute}</td>
                  <td className="actions">
                    <button onClick={() => toggleKeys(c.id)}>
                      {expandedId === c.id ? "Hide keys" : "Keys"}
                    </button>
                    <button onClick={() => startEdit(c)}>Edit</button>
                    <button onClick={() => handleDelete(c.id)}>Delete</button>
                  </td>
                </tr>
              )}

              {expandedId === c.id && (
                <tr>
                  <td colSpan={3}>
                    <div className="keys-panel">
                      {revealedKey && revealedKey.clientId === c.id && (
                        <p className="revealed-key">
                          New key (shown once — save it now):{" "}
                          <code>{revealedKey.key}</code>
                        </p>
                      )}
                      <table>
                        <thead>
                          <tr>
                            <th>ID</th>
                            <th>Created</th>
                            <th>Status</th>
                            <th></th>
                          </tr>
                        </thead>
                        <tbody>
                          {(keysByClient[c.id] || []).map((k) => (
                            <tr key={k.id}>
                              <td>{k.id}</td>
                              <td>{new Date(k.created_at).toLocaleString()}</td>
                              <td>{k.revoked_at ? "revoked" : "active"}</td>
                              <td className="actions">
                                {!k.revoked_at && (
                                  <button onClick={() => handleRevokeKey(c.id, k.id)}>
                                    Revoke
                                  </button>
                                )}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                      <button onClick={() => handleGenerateKey(c.id)}>
                        Generate new key
                      </button>
                    </div>
                  </td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>

      <form className="add-form" onSubmit={handleCreate}>
        <input
          placeholder="client name"
          value={newClient.name}
          onChange={(e) => setNewClient({ ...newClient, name: e.target.value })}
          required
        />
        <input
          type="number"
          placeholder="rate limit / min"
          value={newClient.rate_limit_per_minute}
          onChange={(e) =>
            setNewClient({ ...newClient, rate_limit_per_minute: e.target.value })
          }
          required
        />
        <button type="submit">Add client</button>
      </form>
    </section>
  );
}
