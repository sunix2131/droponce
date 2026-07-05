import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import { expect, test } from "vitest";
import App from "./App";

test("create transfer button is disabled until a file is selected", async () => {
  window.go = {
    main: {
      App: {
        SelectFile: async () => ({ path: "", name: "", sizeBytes: 0, modifiedAt: "", isSymlink: false }),
        SelectFilesAsZip: async () => ({ path: "", name: "", sizeBytes: 0, modifiedAt: "", isSymlink: false }),
        GetAvailableNetworkEndpoints: async () => [],
        CreateTransfer: async () => {
          throw new Error("should not be called");
        },
        CreateInternetTransfer: async () => {
          throw new Error("should not be called");
        },
        CreateCloudPubTransfer: async () => {
          throw new Error("should not be called");
        },
        CreateDirectTransfer: async () => {
          throw new Error("should not be called");
        },
        AcceptDirectTransfer: async () => ({
          sessionId: "s1",
          status: "connecting",
          bytesReceived: 0,
          startedAt: new Date().toISOString(),
        }),
        ListIncomingTransfers: async () => [],
        CancelDirectSession: async () => undefined,
        GetTransferLimits: async () => ({
          localSingleFileLimitLabel: "Без лимита приложения.",
          internetSingleFileLimitGb: 50,
          internetSingleFileLimitText: "До 50 ГБ.",
          multiFileSupported: false,
          multiFileAdvice: "Одна ссылка передаёт один файл.",
        }),
        GetRecommendedRelayURL: async () => ({ url: "http://192.168.1.10:8088", isLocalLan: true }),
        ListActiveTransfers: async () => [],
        GetTransfer: async () => {
          throw new Error("not used");
        },
        CancelTransfer: async () => undefined,
        GetTransferQRCode: async () => ({ pngBase64: "" }),
        SaveTransferQRCode: async () => ({ path: "" }),
        CopyTransferLink: async () => undefined,
        RevealTransferSourceFile: async () => undefined,
        ListTransferHistory: async () => [],
        DeleteHistoryItem: async () => undefined,
        ClearTransferHistory: async () => undefined,
        GetSettings: async () => ({
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
        }),
        UpdateSettings: async (settings) => settings,
        GetDiagnostics: async () => ({
          version: "0.1.0",
          goVersion: "go",
          wailsVersion: "wails",
          sqlitePath: "",
          logsPath: "",
          activeServerCount: 0,
          activeTransferCount: 0,
        }),
      },
    },
  };

  render(<App />);
  expect(await screen.findByRole("button", { name: /Создать QR-ссылку/i })).toBeDisabled();
});
