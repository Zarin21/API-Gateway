import { useEffect, useRef, useState } from "react";
import "./App.css";
import { GATEWAY_URL } from "./api";
import RoutesPanel from "./RoutesPanel";
import ClientsPanel from "./ClientsPanel";

const MAX_EVENTS = 50;

function App() {
  const [token, setToken] = useState(
    () => localStorage.getItem("adminToken") || ""
  );
  const [tokenInput, setTokenInput] = useState("");
  const [wsStatus, setWsStatus] = useState("disconnected");
  const [events, setEvents] = useState([]);

  const wsRef = useRef(null);

  function connect(newToken) {
    localStorage.setItem("adminToken", newToken);
    setToken(newToken);
  }

  function disconnect() {
    localStorage.removeItem("adminToken");
    setToken("");
    setEvents([]);
  }

  // Live traffic feed. Browsers can't set custom headers on a WebSocket
  // handshake, so the token travels as a query parameter here instead of
  // the X-Admin-Token header RoutesPanel/ClientsPanel use for their REST
  // calls.
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

      <RoutesPanel token={token} />
      <ClientsPanel token={token} />
    </div>
  );
}

export default App;
