import { useRef, useEffect, useState, useMemo } from "react";
import type { ArdennEvent } from "@/types/ardenn";

interface EventLogProps {
  events: ArdennEvent[];
}

const EVENT_TYPE_COLORS: Record<string, string> = {
  "run.": "text-blue-600",
  "step.": "text-emerald-600",
  "eval.": "text-purple-600",
  "gate.": "text-yellow-600",
  "hand.": "text-orange-600",
  "constraint.": "text-red-600",
  "continuity.": "text-cyan-600",
};

function getEventColor(eventType: string): string {
  for (const [prefix, color] of Object.entries(EVENT_TYPE_COLORS)) {
    if (eventType.startsWith(prefix)) return color;
  }
  return "text-muted-foreground";
}

const EVENT_TYPE_PREFIXES = [
  { value: "", label: "All" },
  { value: "run.", label: "Run" },
  { value: "step.", label: "Step" },
  { value: "eval.", label: "Eval" },
  { value: "gate.", label: "Gate" },
  { value: "hand.", label: "Hand" },
  { value: "constraint.", label: "Constraint" },
  { value: "continuity.", label: "Continuity" },
];

export function EventLog({ events }: EventLogProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [filter, setFilter] = useState("");

  const filtered = useMemo(
    () =>
      filter
        ? events.filter((e) => e.event_type.startsWith(filter))
        : events,
    [events, filter],
  );

  // Auto-scroll to bottom on new events
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filtered.length, autoScroll]);

  // Detect user scroll to disable auto-scroll
  const handleScroll = () => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    setAutoScroll(isAtBottom);
  };

  return (
    <div className="rounded-lg border bg-card flex flex-col h-full">
      {/* Header + filter */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <h3 className="text-sm font-semibold text-muted-foreground">
          Event Log ({events.length})
        </h3>
        <div className="flex gap-1">
          {EVENT_TYPE_PREFIXES.map((p) => (
            <button
              key={p.value}
              onClick={() => setFilter(p.value)}
              className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors ${
                filter === p.value
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {/* Event list */}
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overscroll-contain p-2 space-y-0.5 font-mono text-xs"
      >
        {filtered.map((e) => (
          <div
            key={e.id}
            className="flex items-start gap-2 py-0.5 hover:bg-muted/30 rounded px-1"
          >
            <span className="shrink-0 text-muted-foreground w-8 text-right">
              #{e.sequence}
            </span>
            <span className="shrink-0 text-muted-foreground w-16">
              {new Date(e.created_at).toLocaleTimeString()}
            </span>
            <span className={`shrink-0 font-medium ${getEventColor(e.event_type)}`}>
              {e.event_type}
            </span>
            <span className="text-muted-foreground truncate">
              {e.actor_type}
              {e.step_id ? ` step:${e.step_id.substring(0, 8)}` : ""}
              {Object.keys(e.payload).length > 0
                ? ` ${JSON.stringify(e.payload).substring(0, 80)}`
                : ""}
            </span>
          </div>
        ))}
        {filtered.length === 0 && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            No events{filter ? ` matching "${filter}"` : ""}
          </div>
        )}
      </div>

      {/* Auto-scroll indicator */}
      {!autoScroll && (
        <button
          onClick={() => {
            setAutoScroll(true);
            if (scrollRef.current) {
              scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
            }
          }}
          className="border-t px-3 py-1.5 text-[10px] text-center text-primary hover:bg-muted"
        >
          New events available — click to scroll to bottom
        </button>
      )}
    </div>
  );
}
