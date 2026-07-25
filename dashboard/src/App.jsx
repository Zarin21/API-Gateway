import { useEffect, useRef, useState } from "react";
import "./App.css";

const GATEWAY_URL = "http://localhost:8080";
const MAX_EVENTS = 50;

function App() {
  const [token, setToken] = useState(
    () => localStorage.getItem("adminToken") || ""
  );
  const [tokenInput, setTokenInput] = useState("");
  const [wsStatus, setWsStatus] = useState("disconnected");
  const [events, setEvents] = useState([]);
  const [routes, setRoutes] = useState([]);
  const [clients, setClients] = useState([]);
  const [error, setError] = useState("");

  const wsRef = useRef(null);

  function connect(newToken) {
    localStorage.setItem("adminToken", newToken);
    setToken(newToken);
  }

  function disconnect() {
    localStorage.removeItem("adminToken");
    setToken("");
    setEvents([]);
    setRoutes([]);
    setClients([]);
  }

  // Pull current config over the admin REST API. This only works if the
  // gateway's CORS middleware allows this page's origin — if it's
  // misconfigured, these fetches fail with a CORS error in the console
  // before the request even reaches our code.
  useEffect(() => {
    if (!token) return;

    const headers = { "X-Admin-Token": token };

    Promise.all([
      fetch(`${GATEWAY_URL}/admin/routes/`, { headers }),
      fetch(`${GATEWAY_URL}/admin/clients/`, { headers }),
    ])
      .then(async ([routesRes, clientsRes]) => {
        if (!routesRes.ok || !clientsRes.ok) {
          throw new Error("admin token rejected");
        }
        setRoutes(await routesRes.json());
        setClients(await clientsRes.json());
        setError("");
      })
      .catch(() => {
        setError("Failed to load config — check the admin token.");
      });
  }, [token]);

  // Live traffic feed. Browsers can't set custom headers on a WebSocket
  // handshake, so the token travels as a query parameter here instead of
  // the X-Admin-Token header the REST calls above use.
  useEffect(() => {
    if (!token) return;

    const wsURL = `${GATEWAY_URL.replace("http", "ws")}/admin/ws?token=${encodeURIComponent(token)}`;
    const ws = new WebSocket(wsURL);
    wsRef.current = ws;

    ws.onopen = () => setWsStatus("connected");
    ws.onclose = () => setWsStatus("disconnected");
    ws.onerror = () => setWsStatus("error");

    ws.onmessage = (event) => {
      const parsed = JSON.parse(event.data);
      setEvents((prev) => [parsed, ...prev].slice(0, MAX_EVENTS));
    };

    return () => ws.close();
  }, [token]);

  if (!token) {
    return (
      <div className="login">
        <h1>
          API Gateway <span className="title-sub">Dashboard</span>
        </h1>
        <p>Enter the admin token to connect.</p>
        <input
          type="password"
          value={tokenInput}
          onChange={(e) => setTokenInput(e.target.value)}
          placeholder="X-Admin-Token"
        />
        <button onClick={() => connect(tokenInput)} disabled={!tokenInput}>
          Connect
        </button>
      </div>
    );
  }

  return (
    <div className="dashboard">
      <header>
        <h1>
          API Gateway <span className="title-sub">Dashboard</span>
        </h1>
        <span className={`status status-${wsStatus}`}>{wsStatus}</span>
        <button onClick={disconnect}>Disconnect</button>
      </header>

      {error && <p className="error">{error}</p>}

      <section>
        <h2>Live Traffic</h2>
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>Client</th>
              <th>Method</th>
              <th>Path</th>
              <th>Status</th>
              <th>Latency</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e, i) => (
              <tr key={i} className={e.status_code >= 400 ? "row-error" : ""}>
                <td>{new Date(e.timestamp).toLocaleTimeString()}</td>
                <td>{e.client_id ?? "—"}</td>
                <td>{e.method}</td>
                <td>{e.path}</td>
                <td>{e.status_code}</td>
                <td>{e.latency_ms}ms</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section>
        <h2>Routes</h2>
        <table>
          <thead>
            <tr>
              <th>Path Prefix</th>
              <th>Backend URL</th>
            </tr>
          </thead>
          <tbody>
            {routes.map((r) => (
              <tr key={r.id}>
                <td>{r.path_prefix}</td>
                <td>{r.backend_url}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section>
        <h2>Clients</h2>
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Rate Limit / min</th>
            </tr>
          </thead>
          <tbody>
            {clients.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td>{c.rate_limit_per_minute}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}

export default App;
