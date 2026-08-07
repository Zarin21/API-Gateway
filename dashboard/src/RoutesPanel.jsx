import { useEffect, useState } from "react";
import { adminFetch } from "./api";

export default function RoutesPanel({ token }) {
  const [routes, setRoutes] = useState([]);
  const [error, setError] = useState("");
  const [newRoute, setNewRoute] = useState({ path_prefix: "", backend_url: "" });
  const [editingId, setEditingId] = useState(null);
  const [editRoute, setEditRoute] = useState({ path_prefix: "", backend_url: "" });

  async function load() {
    try {
      setRoutes(await adminFetch(token, "/admin/routes/"));
      setError("");
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    load();
  }, [token]);

  async function handleCreate(e) {
    e.preventDefault();
    try {
      await adminFetch(token, "/admin/routes/", {
        method: "POST",
        body: JSON.stringify(newRoute),
      });
      setNewRoute({ path_prefix: "", backend_url: "" });
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  function startEdit(route) {
    setEditingId(route.id);
    setEditRoute({ path_prefix: route.path_prefix, backend_url: route.backend_url });
  }

  async function handleSaveEdit(id) {
    try {
      await adminFetch(token, `/admin/routes/${id}`, {
        method: "PUT",
        body: JSON.stringify(editRoute),
      });
      setEditingId(null);
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleDelete(id) {
    if (!confirm("Delete this route?")) return;
    try {
      await adminFetch(token, `/admin/routes/${id}`, { method: "DELETE" });
      await load();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <section>
      <h2>Routes</h2>
      {error && <p className="error">{error}</p>}
      <table>
        <thead>
          <tr>
            <th>Path Prefix</th>
            <th>Backend URL</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {routes.map((r) =>
            editingId === r.id ? (
              <tr key={r.id}>
                <td>
                  <input
                    value={editRoute.path_prefix}
                    onChange={(e) =>
                      setEditRoute({ ...editRoute, path_prefix: e.target.value })
                    }
                  />
                </td>
                <td>
                  <input
                    value={editRoute.backend_url}
                    onChange={(e) =>
                      setEditRoute({ ...editRoute, backend_url: e.target.value })
                    }
                  />
                </td>
                <td className="actions">
                  <button onClick={() => handleSaveEdit(r.id)}>Save</button>
                  <button onClick={() => setEditingId(null)}>Cancel</button>
                </td>
              </tr>
            ) : (
              <tr key={r.id}>
                <td>{r.path_prefix}</td>
                <td>{r.backend_url}</td>
                <td className="actions">
                  <button onClick={() => startEdit(r)}>Edit</button>
                  <button onClick={() => handleDelete(r.id)}>Delete</button>
                </td>
              </tr>
            )
          )}
        </tbody>
      </table>

      <form className="add-form" onSubmit={handleCreate}>
        <input
          placeholder="/path-prefix"
          value={newRoute.path_prefix}
          onChange={(e) => setNewRoute({ ...newRoute, path_prefix: e.target.value })}
          required
        />
        <input
          placeholder="http://backend:port"
          value={newRoute.backend_url}
          onChange={(e) => setNewRoute({ ...newRoute, backend_url: e.target.value })}
          required
        />
        <button type="submit">Add route</button>
      </form>
    </section>
  );
}
