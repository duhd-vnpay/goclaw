import { useState, useCallback, useRef } from "react";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { useAuthStore } from "@/stores/use-auth-store";
import { Methods, Events } from "@/api/protocol";

// --- Types ---

export type PartyMode = "standard" | "deep" | "token_ring";

export interface PersonaInfo {
  key: string;
  emoji: string;
  name: string;
  role: string;
  color: string;
}

export interface PartyMessage {
  id: string;
  type: "intro" | "spoke" | "thinking" | "round_header" | "context" | "summary" | "artifact";
  personaKey?: string;
  personaEmoji?: string;
  personaName?: string;
  content: string;
  round?: number;
  mode?: PartyMode;
  timestamp: number;
}

export interface PartySession {
  id: string;
  topic: string;
  status: "active" | "closed";
  personas: PersonaInfo[];
  round: number;
  mode: PartyMode;
  createdAt: string;
}

export interface PartySummary {
  agreements?: string[];
  disagreements?: string[];
  decisions?: string[];
  actionItems?: string[];
  compliance?: string[];
  markdown?: string;
}

// Persona color palette for left-border styling
const PERSONA_COLORS = [
  "#3b82f6", "#ef4444", "#10b981", "#f59e0b", "#8b5cf6",
  "#ec4899", "#06b6d4", "#f97316", "#6366f1", "#14b8a6",
  "#e11d48", "#84cc16", "#a855f7", "#0ea5e9",
];

export function useParty() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);

  const [sessions, setSessions] = useState<PartySession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [messages, setMessages] = useState<PartyMessage[]>([]);
  const [personas, setPersonas] = useState<PersonaInfo[]>([]);
  const [thinkingPersonas, setThinkingPersonas] = useState<Set<string>>(new Set());
  const [round, setRound] = useState(0);
  const [mode, setMode] = useState<PartyMode>("standard");
  const [status, setStatus] = useState<"idle" | "active" | "closed">("idle");
  const [summary, setSummary] = useState<PartySummary | null>(null);
  const [loading, setLoading] = useState(false);

  const msgIdCounter = useRef(0);
  const personaColorMap = useRef<Map<string, string>>(new Map());

  const getPersonaColor = useCallback((key: string): string => {
    if (personaColorMap.current.has(key)) {
      return personaColorMap.current.get(key)!;
    }
    const idx = personaColorMap.current.size % PERSONA_COLORS.length;
    const color = PERSONA_COLORS[idx] ?? "#3b82f6";
    personaColorMap.current.set(key, color);
    return color;
  }, []);

  const addMessage = useCallback((msg: Omit<PartyMessage, "id" | "timestamp">) => {
    const id = `pm-${++msgIdCounter.current}`;
    setMessages((prev) => [...prev, { ...msg, id, timestamp: Date.now() }]);
  }, []);

  // --- Event handlers ---

  const handlePartyStarted = useCallback((payload: unknown) => {
    const p = payload as {
      sessionId: string;
      topic: string;
      personas: Array<{ key: string; emoji: string; name: string; role: string }>;
    };
    setActiveSessionId(p.sessionId);
    const enriched = p.personas.map((pe) => ({
      ...pe,
      color: getPersonaColor(pe.key),
    }));
    setPersonas(enriched);
    setRound(0);
    setMode("standard");
    setStatus("active");
    setSummary(null);
    setMessages([]);
    personaColorMap.current.clear();
    enriched.forEach((pe) => personaColorMap.current.set(pe.key, pe.color));
  }, [getPersonaColor]);

  const handlePersonaIntro = useCallback((payload: unknown) => {
    const p = payload as { personaKey: string; emoji: string; name: string; intro: string };
    addMessage({
      type: "intro",
      personaKey: p.personaKey,
      personaEmoji: p.emoji,
      personaName: p.name,
      content: p.intro,
    });
  }, [addMessage]);

  const handleRoundStarted = useCallback((payload: unknown) => {
    const p = payload as { round: number; mode: string };
    setRound(p.round);
    setMode(p.mode as PartyMode);
    addMessage({
      type: "round_header",
      content: "",
      round: p.round,
      mode: p.mode as PartyMode,
    });
  }, [addMessage]);

  const handlePersonaThinking = useCallback((payload: unknown) => {
    const p = payload as { personaKey: string };
    setThinkingPersonas((prev) => new Set(prev).add(p.personaKey));
  }, []);

  const handlePersonaSpoke = useCallback((payload: unknown) => {
    const p = payload as { personaKey: string; emoji: string; name: string; message: string; round: number };
    setThinkingPersonas((prev) => {
      const next = new Set(prev);
      next.delete(p.personaKey);
      return next;
    });
    addMessage({
      type: "spoke",
      personaKey: p.personaKey,
      personaEmoji: p.emoji,
      personaName: p.name,
      content: p.message,
      round: p.round,
    });
  }, [addMessage]);

  const handleRoundComplete = useCallback((payload: unknown) => {
    const p = payload as { round: number };
    setThinkingPersonas(new Set());
    void p;
  }, []);

  const handleContextAdded = useCallback((payload: unknown) => {
    const p = payload as { type: string; name?: string };
    addMessage({
      type: "context",
      content: `Context added: ${p.type}${p.name ? ` (${p.name})` : ""}`,
    });
  }, [addMessage]);

  const handleSummaryReady = useCallback((payload: unknown) => {
    const p = payload as PartySummary;
    setSummary(p);
    addMessage({
      type: "summary",
      content: p.markdown ?? "",
    });
  }, [addMessage]);

  const handleArtifact = useCallback((payload: unknown) => {
    const p = payload as { name: string; content: string };
    addMessage({
      type: "artifact",
      content: `**${p.name}**\n\n${p.content}`,
    });
  }, [addMessage]);

  const handlePartyClosed = useCallback((_payload: unknown) => {
    setStatus("closed");
    setThinkingPersonas(new Set());
  }, []);

  // --- Subscribe to events ---
  useWsEvent(Events.PARTY_STARTED, handlePartyStarted);
  useWsEvent(Events.PARTY_PERSONA_INTRO, handlePersonaIntro);
  useWsEvent(Events.PARTY_ROUND_STARTED, handleRoundStarted);
  useWsEvent(Events.PARTY_PERSONA_THINKING, handlePersonaThinking);
  useWsEvent(Events.PARTY_PERSONA_SPOKE, handlePersonaSpoke);
  useWsEvent(Events.PARTY_ROUND_COMPLETE, handleRoundComplete);
  useWsEvent(Events.PARTY_CONTEXT_ADDED, handleContextAdded);
  useWsEvent(Events.PARTY_SUMMARY_READY, handleSummaryReady);
  useWsEvent(Events.PARTY_ARTIFACT, handleArtifact);
  useWsEvent(Events.PARTY_CLOSED, handlePartyClosed);

  // --- RPC calls ---

  const listSessions = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    try {
      const res = await ws.call<{ sessions: PartySession[] }>(Methods.PARTY_LIST);
      setSessions(res.sessions ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [ws, connected]);

  const startParty = useCallback(
    async (topic: string, teamPreset?: string, personaKeys?: string[]) => {
      if (!connected) return;
      setLoading(true);
      try {
        await ws.call(Methods.PARTY_START, {
          topic,
          teamPreset: teamPreset ?? undefined,
          personaKeys: personaKeys ?? undefined,
        });
      } catch {
        // ignore
      } finally {
        setLoading(false);
      }
    },
    [ws, connected],
  );

  const runRound = useCallback(
    async (sessionId: string, roundMode?: PartyMode) => {
      await ws.call(Methods.PARTY_ROUND, {
        sessionId,
        mode: roundMode ?? undefined,
      });
    },
    [ws],
  );

  const askQuestion = useCallback(
    async (sessionId: string, text: string) => {
      await ws.call(Methods.PARTY_QUESTION, { sessionId, text });
    },
    [ws],
  );

  const addContext = useCallback(
    async (sessionId: string, type: string, name?: string, content?: string) => {
      await ws.call(Methods.PARTY_ADD_CONTEXT, { sessionId, type, name, content });
    },
    [ws],
  );

  const getSummary = useCallback(
    async (sessionId: string) => {
      await ws.call(Methods.PARTY_SUMMARY, { sessionId });
    },
    [ws],
  );

  const exitParty = useCallback(
    async (sessionId: string) => {
      await ws.call(Methods.PARTY_EXIT, { sessionId });
    },
    [ws],
  );

  return {
    // state
    sessions,
    activeSessionId,
    messages,
    personas,
    thinkingPersonas,
    round,
    mode,
    status,
    summary,
    loading,

    // actions
    listSessions,
    startParty,
    runRound,
    askQuestion,
    addContext,
    getSummary,
    exitParty,
    setActiveSessionId,
    getPersonaColor,
  };
}
