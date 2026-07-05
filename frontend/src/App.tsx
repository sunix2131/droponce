import { useEffect, useState } from "react";
import {
  Clock,
  Copy,
  Download,
  FileUp,
  FolderOpen,
  History,
  Languages,
  Moon,
  QrCode,
  RefreshCw,
  Save,
  Send,
  Settings as SettingsIcon,
  ShieldAlert,
  Smartphone,
  Sun,
  Trash2,
  Wifi,
  X,
} from "lucide-react";
import { api, FileSelection, IncomingTransfer, NetworkEndpoint, Settings, TransferDetails } from "./api/wails";
import type { TransferLimits } from "./api/wails";
import { messages } from "./i18n/messages";
import { formatBytes, formatDate } from "./lib/format";
import "./styles/app.css";

type View = "send" | "active" | "history" | "settings";
type SendMode = "local" | "cloudpub" | "internet" | "direct";

const defaultSettings: Settings = {
  language: "ru",
  theme: "system",
  defaultRelayUrl: "",
  defaultExpiryMinutes: 30,
  defaultMaxDownloads: 1,
  defaultCalculateSha: true,
  warnTrustedLocalOnly: true,
  maxActiveTransfers: 10,
  confirmCloseWithLinks: true,
  cloudPubToken: "",
};

function relayIssue(value: string): string {
  if (!value.trim()) return "Укажите публичный HTTPS relay.";
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "https:") return "Для QR вне одной сети нужен HTTPS relay.";
    const host = parsed.hostname.toLowerCase();
    if (host === "localhost" || host === "127.0.0.1" || host === "::1") return "Локальный relay не откроется вне этой сети.";
    if (/^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[0-1])\.|169\.254\.)/.test(host)) return "Адрес локальной сети не откроется через интернет.";
    return "";
  } catch {
    return "Введите корректный URL relay.";
  }
}

function App() {
  const [view, setView] = useState<View>("send");
  const [settings, setSettings] = useState<Settings>(defaultSettings);
  const [file, setFile] = useState<FileSelection | null>(null);
  const [limits, setLimits] = useState<TransferLimits | null>(null);
  const [sendMode, setSendMode] = useState<SendMode>("local");
  const [endpoints, setEndpoints] = useState<NetworkEndpoint[]>([]);
  const [bindIp, setBindIp] = useState("");
  const [relayUrl, setRelayUrl] = useState("");
  const [brokerUrl, setBrokerUrl] = useState("http://localhost:8091");
  const [directTicket, setDirectTicket] = useState("");
  const [incoming, setIncoming] = useState<IncomingTransfer[]>([]);
  const [recipientId, setRecipientId] = useState("");
  const [expiry, setExpiry] = useState(30);
  const [downloads, setDownloads] = useState(1);
  const [hash, setHash] = useState(true);
  const [isCreating, setIsCreating] = useState(false);
  const [active, setActive] = useState<TransferDetails[]>([]);
  const [history, setHistory] = useState<TransferDetails[]>([]);
  const [selected, setSelected] = useState<TransferDetails | null>(null);
  const [qr, setQr] = useState("");
  const [error, setError] = useState("");
  const t = messages[settings.language] ?? messages.ru;

  useEffect(() => {
    void refreshAll();
    void api.settings().then((next) => {
      setSettings(next);
      setExpiry(next.defaultExpiryMinutes);
      setDownloads(next.defaultMaxDownloads);
      setHash(next.defaultCalculateSha);
      setRelayUrl(next.defaultRelayUrl || "");
    }).catch(() => undefined);
    void api.limits().then(setLimits).catch(() => undefined);
    // Runs once on startup; refreshAll intentionally captures initial bindIp.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = settings.theme;
    document.documentElement.lang = settings.language;
  }, [settings]);

  useEffect(() => {
    if (!selected) return;
    void api.qr(selected.id).then((value) => setQr(value.pngBase64)).catch(() => setQr(""));
  }, [selected]);

  async function refreshAll() {
    const [nets, activeTransfers, pastTransfers] = await Promise.all([
      api.endpoints().catch(() => []),
      api.activeTransfers().catch(() => []),
      api.history().catch(() => []),
    ]);
    setEndpoints(nets);
    setActive(activeTransfers);
    setHistory(pastTransfers);
    if (!bindIp && nets[0]) setBindIp(nets[0].ipAddress);
    setIncoming(await api.incomingTransfers().catch(() => []));
  }

  async function chooseFile() {
    setError("");
    try {
      setFile(await api.selectFile());
    } catch (err) {
      setError(String(err));
    }
  }

  async function chooseFilesAsZip() {
    setError("");
    try {
      setFile(await api.selectFilesAsZip());
    } catch (err) {
      setError(String(err));
    }
  }

  async function createTransfer() {
    if (!file) return;
    setError("");
    setIsCreating(true);
    try {
      const transfer = sendMode === "direct"
        ? await api.createDirectTransfer({
          sourcePath: file.path,
          brokerUrl,
          expiresInMinutes: expiry,
          maxDownloads: 1,
          calculateHash: hash,
        })
        : sendMode === "cloudpub"
          ? await api.createCloudPubTransfer({
          sourcePath: file.path,
          bindIp,
          expiresInMinutes: expiry,
          maxDownloads: downloads,
          calculateHash: hash,
        })
        : sendMode === "internet"
          ? await api.createInternetTransfer({
          sourcePath: file.path,
          relayUrl,
          recipientId,
          expiresInMinutes: expiry,
          maxDownloads: downloads,
          calculateHash: hash,
        })
          : await api.createTransfer({
          sourcePath: file.path,
          bindIp,
          expiresInMinutes: expiry,
          maxDownloads: downloads,
          calculateHash: hash,
        });
      setSelected(transfer);
      setView("active");
      await refreshAll();
    } catch (err) {
      setError(String(err));
    } finally {
      setIsCreating(false);
    }
  }

  async function cancelTransfer(id: string) {
    await api.cancel(id);
    if (selected?.id === id) setSelected(null);
    await refreshAll();
  }

  async function acceptDirectTicket() {
    setError("");
    try {
      await api.acceptDirectTransfer(directTicket);
      setDirectTicket("");
      await refreshAll();
    } catch (err) {
      setError(String(err));
    }
  }

  const currentRelayIssue = sendMode === "internet" ? relayIssue(relayUrl) : "";
  const canCreate = Boolean(file && expiry >= 1 && expiry <= 1440 && (sendMode === "direct" ? brokerUrl : sendMode === "cloudpub" ? bindIp && settings.cloudPubToken.trim() : sendMode === "internet" ? !currentRelayIssue : bindIp));

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand"><QrCode size={28} /><span>DropOnce</span></div>
        <button className={view === "send" ? "active" : ""} onClick={() => setView("send")}><Send size={18} />{t.navSend}</button>
        <button className={view === "active" ? "active" : ""} onClick={() => setView("active")}><Download size={18} />{t.navActive}</button>
        <button className={view === "history" ? "active" : ""} onClick={() => setView("history")}><History size={18} />{t.navHistory}</button>
        <button className={view === "settings" ? "active" : ""} onClick={() => setView("settings")}><SettingsIcon size={18} />{t.navSettings}</button>
        <div className="sidebarFooter">
          <button onClick={() => setSettings({ ...settings, theme: settings.theme === "dark" ? "light" : "dark" })}>{settings.theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}Theme</button>
          <button onClick={() => setSettings({ ...settings, language: settings.language === "ru" ? "en" : "ru" })}><Languages size={17} />{settings.language.toUpperCase()}</button>
          <span>v0.1.0</span>
        </div>
      </aside>
      <main className="content">
        {error && <div className="alert"><ShieldAlert size={18} />{error}</div>}
        {view === "send" && (
          <section className="sendPage">
            <div className="intro">
              <div>
                <span className="eyebrow">DropOnce</span>
                <h1>Передать файл на телефон по QR</h1>
                <p>{sendMode === "direct" ? "Для DropOnce на другом устройстве: файл шифруется end-to-end и идёт через зашифрованный мост." : sendMode === "cloudpub" ? "Телефон открывает HTTPS QR в браузере из любой сети, а файл стримится прямо с этого компьютера через CloudPub." : sendMode === "internet" ? "Телефон открывает QR в браузере и скачивает файл из любой сети через публичный HTTPS relay." : "Телефон открывает QR в браузере, без установки приложения, если он в той же Wi‑Fi / Ethernet сети."}</p>
              </div>
              <div className="intentStrip" aria-label="Transfer flow">
                <span className={file ? "done" : ""}><FileUp size={18} />Файл</span>
                <span className={(sendMode === "direct" ? brokerUrl : sendMode === "cloudpub" ? settings.cloudPubToken && bindIp : sendMode === "internet" ? relayUrl : bindIp) ? "done" : ""}><Wifi size={18} />{sendMode === "direct" ? "Broker" : sendMode === "cloudpub" ? "CloudPub" : sendMode === "internet" ? "Relay" : "Локальная сеть"}</span>
                <span className={canCreate ? "done" : ""}><QrCode size={18} />QR-ссылка</span>
                <span><Smartphone size={18} />{sendMode === "direct" ? "DropOnce app" : sendMode === "internet" ? "Браузер телефона" : "Браузер телефона"}</span>
              </div>
            </div>
            <div className="modeSwitch" role="tablist" aria-label="Transfer mode">
              <button className={sendMode === "local" ? "active" : ""} onClick={() => setSendMode("local")}><Wifi size={17} />В одной сети</button>
              <button className={sendMode === "cloudpub" ? "active" : ""} onClick={() => setSendMode("cloudpub")}><Smartphone size={17} />CloudPub HTTPS</button>
              <button className={sendMode === "internet" ? "active" : ""} onClick={() => setSendMode("internet")}><Smartphone size={17} />Через интернет</button>
              <button className={sendMode === "direct" ? "active" : ""} onClick={() => setSendMode("direct")}><QrCode size={17} />Direct P2P</button>
            </div>
            <div className="grid two">
              <div className="panel filePanel">
                <div className="panelTitle">
                  <span className="step">1</span>
                  <div>
                    <h2>Файл для передачи</h2>
                    <p>{sendMode === "direct" ? "Один файл шифруется и отправляется в DropOnce получателя." : sendMode === "cloudpub" ? "Один файл отдаётся с этого компьютера через HTTPS-туннель." : sendMode === "internet" ? "Один файл загрузится на публичный relay." : "Один файл, без загрузки в облако."}</p>
                  </div>
                </div>
              <button className="drop" onClick={chooseFile}>
                <FolderOpen size={34} />
                <strong>{file ? file.name : "Выберите файл на этом компьютере"}</strong>
                <span>{file ? `${formatBytes(file.sizeBytes)} · ${file.path}` : "После выбора DropOnce сделает временную QR-ссылку."}</span>
              </button>
              {file?.symlinkWarning && <p className="warning">{file.symlinkWarning}</p>}
              <div className="fileActions">
                <button className="ghost" onClick={chooseFilesAsZip}><FileUp size={17} />Несколько файлов в ZIP</button>
                {file && <button className="ghost" onClick={() => setFile(null)}><Trash2 size={17} />Удалить выбранный файл</button>}
              </div>
            </div>
              <div className="panel">
                <div className="panelTitle">
                  <span className="step">2</span>
                  <div>
                    <h2>Как будет работать ссылка</h2>
                    <p>{sendMode === "direct" ? "QR содержит pairing-ticket. Получатель открывает его в DropOnce." : sendMode === "cloudpub" ? "DropOnce сам поднимет CloudPub-туннель и даст HTTPS QR." : sendMode === "internet" ? "Откроется как мобильная веб-страница через relay." : "Откроется как мобильная веб-страница в выбранной локальной сети."}</p>
                  </div>
                </div>
              <div className="infoGrid">
                <div className="infoCard">
                  <strong>{sendMode === "internet" ? "Размер" : "Большие файлы"}</strong>
                  <span>{sendMode === "direct" ? "Файл идёт чанками через encrypted bridge; broker не хранит файл на диске." : sendMode === "cloudpub" ? "Без лимита приложения: файл стримится с диска через HTTPS-туннель." : sendMode === "internet" ? limits?.internetSingleFileLimitText ?? "Стандартный relay принимает до 50 ГБ на файл." : limits?.localSingleFileLimitLabel ?? "Без лимита приложения."}</span>
                </div>
                <div className="infoCard">
                  <strong>Много файлов</strong>
                  <span>{limits?.multiFileAdvice ?? "Сейчас одна ссылка передаёт один файл. Для нескольких файлов используйте .zip."}</span>
                </div>
              </div>
              {sendMode === "local" || sendMode === "cloudpub" ? (
                <>
                  <label>Локальный адрес</label>
                  <select value={bindIp} onChange={(event) => setBindIp(event.target.value)}>
                    {endpoints.map((endpoint) => <option key={endpoint.ipAddress} value={endpoint.ipAddress}>{endpoint.displayName}</option>)}
                  </select>
                  <p className="muted">{sendMode === "cloudpub" ? (settings.cloudPubToken.trim() ? "DropOnce запустит локальный сервер и опубликует его через CloudPub HTTPS." : "Добавьте CloudPub token в настройках, чтобы создать публичный HTTPS QR.") : endpoints.length ? "Подобран автоматически. Получатель откроет QR в браузере, приложение само запустит локальный сервер на этом адресе." : "Не удалось найти частную локальную сеть."}</p>
                </>
              ) : sendMode === "internet" ? (
                <>
                  <label>Публичный HTTPS relay</label>
                  <input className="textInput" value={relayUrl} onChange={(event) => setRelayUrl(event.target.value)} placeholder="https://relay.example.com" />
                  <p className="muted">QR откроется в браузере телефона вне вашей сети только если relay доступен из интернета.</p>
                  {currentRelayIssue && <p className="warning compact">{currentRelayIssue}</p>}
                  <label>ID получателя</label>
                  <input className="textInput" value={recipientId} onChange={(event) => setRecipientId(event.target.value)} placeholder="например: alice-phone" />
                </>
              ) : (
                <>
                  <label>Broker / encrypted bridge</label>
                  <input className="textInput" value={brokerUrl} onChange={(event) => setBrokerUrl(event.target.value)} placeholder="http://broker.example.com:8091" />
                  <p className="muted">HTTPS не обязателен: payload шифруется end-to-end. Broker нужен только для встречи устройств и encrypted fallback.</p>
                  <label>Принять Direct ticket</label>
                  <textarea className="textArea" value={directTicket} onChange={(event) => setDirectTicket(event.target.value)} placeholder="droponce://receive/..." />
                  <button className="ghost acceptButton" disabled={!directTicket.trim()} onClick={acceptDirectTicket}><Download size={17} />Принять файл</button>
                </>
              )}
              <div className="row">
                <label>Срок</label>
                <select value={expiry} onChange={(event) => setExpiry(Number(event.target.value))}>
                  {[10, 30, 60, 180, 720, 1440].map((value) => <option key={value} value={value}>{value < 60 ? `${value} мин` : `${value / 60} ч`}</option>)}
                </select>
              </div>
              <div className="row">
                <label>Скачивания</label>
                <select value={downloads} onChange={(event) => setDownloads(Number(event.target.value))}>
                  {[1, 3, 5, 10].map((value) => <option key={value} value={value}>{value}</option>)}
                </select>
              </div>
              <label className="check"><input type="checkbox" checked={hash} onChange={(event) => setHash(event.target.checked)} />Проверять SHA-256</label>
              <div className="securityNote">
                <ShieldAlert size={17} />
                <span>{sendMode === "direct" ? "Broker не должен видеть содержимое: metadata и chunks шифруются ChaCha20-Poly1305 на уровне приложения." : sendMode === "cloudpub" ? "CloudPub даёт публичный HTTPS-доступ к временной ссылке. Токен хранится локально, а передача отключается после лимита, отмены или закрытия приложения." : sendMode === "internet" ? "Используйте свой или доверенный relay: файл хранится там временно до скачивания, отмены или истечения срока." : "Ссылка открывается в браузере телефона, работает только в выбранной локальной сети и отключается после лимита, отмены или истечения срока."}</span>
              </div>
              <button className="primary createButton" disabled={!canCreate || isCreating} onClick={createTransfer}><QrCode size={18} />{isCreating ? "Готовлю передачу..." : sendMode === "direct" ? "Создать Direct QR" : sendMode === "cloudpub" ? "Создать CloudPub HTTPS QR" : sendMode === "internet" ? "Загрузить на relay и создать QR" : t.create}</button>
              {!canCreate && <p className="readyHint">{file ? "Проверьте сеть, relay или CloudPub token." : "Начните с выбора файла."}</p>}
              {canCreate && <p className="readyHint positive">Готово: QR появится на экране активной передачи.</p>}
              </div>
            </div>
          </section>
        )}
        {view === "active" && (
          <ActiveView active={active} incoming={incoming} selected={selected ?? active[0] ?? null} setSelected={setSelected} qr={qr} t={t} onCancel={cancelTransfer} refresh={refreshAll} />
        )}
        {view === "history" && <HistoryView items={history} t={t} refresh={refreshAll} />}
        {view === "settings" && <SettingsView settings={settings} setSettings={setSettings} endpoints={endpoints} />}
      </main>
    </div>
  );
}

function ActiveView({ active, incoming, selected, setSelected, qr, t, onCancel, refresh }: {
  active: TransferDetails[];
  incoming: IncomingTransfer[];
  selected: TransferDetails | null;
  setSelected: (item: TransferDetails) => void;
  qr: string;
  t: typeof messages.ru;
  onCancel: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
}) {
  if (!active.length && !incoming.length) return <div className="empty">{t.emptyActive}</div>;
  return (
    <section className="grid activeGrid">
      <div className="list">
        {active.map((item) => (
          <button key={item.id} className={selected?.id === item.id ? "transfer active" : "transfer"} onClick={() => setSelected(item)}>
            <strong>{item.sourceFileName}</strong>
            <span>{formatBytes(item.sourceSizeBytes)} · {item.status}</span>
          </button>
        ))}
        {incoming.map((item) => (
          <div className="transfer incoming" key={item.sessionId}>
            <strong>{item.fileName || "Direct P2P transfer"}</strong>
            <span>{item.status} · {formatBytes(item.bytesReceived)}{item.sizeBytes ? ` из ${formatBytes(item.sizeBytes)}` : ""}</span>
            {item.savedPath && <span>{item.savedPath}</span>}
            {item.errorMessage && <span>{item.errorMessage}</span>}
          </div>
        ))}
      </div>
      {selected && (
        <div className="panel detail">
          <div className="detailHead">
            <h1>{selected.sourceFileName}</h1>
            <span className="pill">{selected.status}</span>
          </div>
          <div className="stats">
            <span><Clock size={16} />{formatDate(selected.expiresAt)}</span>
            <span>{selected.bindIp}:{selected.port}</span>
            <span>Скачиваний: {selected.completedDownloads} из {selected.maxDownloads}</span>
          </div>
          <div className="qrBox">{qr ? <img src={`data:image/png;base64,${qr}`} alt="QR code" /> : <span>Ссылка больше не работает.</span>}</div>
          <div className="actions">
            <button onClick={() => api.copyLink(selected.id)}><Copy size={17} />{t.copy}</button>
            <button onClick={() => api.saveQr(selected.id)}><Save size={17} />{t.saveQr}</button>
            <button onClick={() => api.reveal(selected.id)}><FolderOpen size={17} />{t.reveal}</button>
            <button onClick={refresh}><RefreshCw size={17} />Обновить</button>
            <button className="danger" onClick={() => onCancel(selected.id)}><X size={17} />{t.cancel}</button>
          </div>
        </div>
      )}
    </section>
  );
}

function HistoryView({ items, t, refresh }: { items: TransferDetails[]; t: typeof messages.ru; refresh: () => Promise<void> }) {
  if (!items.length) return <div className="empty">{t.emptyHistory}</div>;
  return (
    <section className="panel">
      <div className="detailHead"><h1>{t.navHistory}</h1><button onClick={async () => { await api.clearHistory(); await refresh(); }}><Trash2 size={17} />Очистить</button></div>
      <div className="table">
        {items.map((item) => (
          <div className="tableRow" key={item.id}>
            <span>{formatDate(item.createdAt)}</span>
            <strong>{item.sourceFileName}</strong>
            <span>{formatBytes(item.sourceSizeBytes)}</span>
            <span>{item.status}</span>
            <span>{item.completedDownloads}</span>
            <button onClick={async () => { await api.deleteHistory(item.id); await refresh(); }}><Trash2 size={16} /></button>
          </div>
        ))}
      </div>
    </section>
  );
}

function SettingsView({ settings, setSettings, endpoints }: { settings: Settings; setSettings: (settings: Settings) => void; endpoints: NetworkEndpoint[] }) {
  const [diagnostics, setDiagnostics] = useState<string[]>([]);
  useEffect(() => {
    void api.diagnostics().then((d) => setDiagnostics([
      `Go: ${d.goVersion}`,
      `Wails: ${d.wailsVersion}`,
      `SQLite: ${d.sqlitePath}`,
      `Logs: ${d.logsPath}`,
      `Active transfers: ${d.activeTransferCount}`,
    ])).catch(() => undefined);
  }, []);
  const save = async (patch: Partial<Settings>) => {
    const next = { ...settings, ...patch };
    setSettings(await api.updateSettings(next).catch(() => next));
  };
  return (
    <section className="grid two">
      <div className="panel">
        <h1>Настройки</h1>
        <label>Язык</label>
        <select value={settings.language} onChange={(e) => void save({ language: e.target.value as Settings["language"] })}>
          <option value="ru">Русский</option><option value="en">English</option>
        </select>
        <label>Тема</label>
        <select value={settings.theme} onChange={(e) => void save({ theme: e.target.value as Settings["theme"] })}>
          <option value="system">Системная</option><option value="light">Светлая</option><option value="dark">Тёмная</option>
        </select>
        <label>Публичный relay для интернета</label>
        <input className="textInput" value={settings.defaultRelayUrl} onChange={(e) => void save({ defaultRelayUrl: e.target.value })} placeholder="https://relay.example.com" />
        <label>CloudPub token</label>
        <input className="textInput" type="password" value={settings.cloudPubToken} onChange={(e) => void save({ cloudPubToken: e.target.value })} placeholder="suy-..." />
        <label className="check"><input type="checkbox" checked={settings.confirmCloseWithLinks} onChange={(e) => void save({ confirmCloseWithLinks: e.target.checked })} />Подтверждать закрытие при активных передачах</label>
      </div>
      <div className="panel">
        <h2>Диагностика</h2>
        <p className="muted">Текущие сетевые интерфейсы</p>
        {endpoints.map((endpoint) => <p key={endpoint.ipAddress}>{endpoint.displayName}</p>)}
        {diagnostics.map((line) => <p className="mono" key={line}>{line}</p>)}
      </div>
    </section>
  );
}

export default App;
