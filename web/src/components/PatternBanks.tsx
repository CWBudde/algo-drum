import type { TransportState } from "../engine/wasmEngine";
import "./PatternBanks.css";

const BANK_LABELS = ["A", "B", "C", "D"] as const;
const MAX_CHAIN_LENGTH = 16;

interface Props {
  disabled: boolean;
  transport: TransportState;
  selectedBank: number;
  activeBank: number;
  queuedBank: number;
  chainPosition: number;
  chainEnabled: boolean;
  chain: Uint8Array;
  onSelectBank: (bank: number) => void;
  onCopyBank: (destination: number) => void;
  onChainEnabledChange: (enabled: boolean) => void;
  onChainChange: (chain: Uint8Array) => void;
}

function bankLabel(bank: number): string {
  return BANK_LABELS[bank] ?? "?";
}

export default function PatternBanks({
  disabled,
  transport,
  selectedBank,
  activeBank,
  queuedBank,
  chainPosition,
  chainEnabled,
  chain,
  onSelectBank,
  onCopyBank,
  onChainEnabledChange,
  onChainChange,
}: Props) {
  const stopped = transport === "stopped";
  const updateEntry = (index: number, bank: number) => {
    const next = chain.slice();
    next[index] = bank;
    onChainChange(next);
  };
  const removeEntry = (index: number) => {
    if (chain.length <= 1) return;
    onChainChange(Uint8Array.from(chain).filter((_, entry) => entry !== index));
  };

  const status = chainEnabled
    ? `Chain position ${chainPosition + 1}: bank ${bankLabel(activeBank)}`
    : queuedBank >= 0
      ? `Bank ${bankLabel(queuedBank)} queued; bank ${bankLabel(activeBank)} is audible`
      : `Bank ${bankLabel(activeBank)} is audible`;

  return (
    <section className="dm-banks" aria-label="Pattern banks and chain">
      <div className="dm-bank-buttons" role="group" aria-label="Pattern banks">
        {BANK_LABELS.map((label, bank) => (
          <button
            key={label}
            type="button"
            className="dm-bank-btn"
            data-active={bank === activeBank || undefined}
            data-queued={bank === queuedBank || undefined}
            aria-pressed={bank === selectedBank}
            aria-current={bank === activeBank ? "true" : undefined}
            disabled={disabled}
            onClick={() => onSelectBank(bank)}
            title={
              chainEnabled
                ? `Edit bank ${label}`
                : transport === "stopped"
                  ? `Select bank ${label}`
                  : `Edit and queue bank ${label} for the next loop`
            }
          >
            {label}
          </button>
        ))}
      </div>

      <details className="dm-bank-copy">
        <summary className="dm-bank-tool">COPY</summary>
        <div
          className="dm-bank-menu"
          role="group"
          aria-label="Copy selected bank"
        >
          {BANK_LABELS.map((label, bank) => (
            <button
              key={label}
              type="button"
              disabled={disabled || bank === selectedBank}
              onClick={(event) => {
                onCopyBank(bank);
                event.currentTarget.closest("details")?.removeAttribute("open");
              }}
              aria-label={`Copy bank ${bankLabel(selectedBank)} to bank ${label}`}
            >
              {bankLabel(selectedBank)} → {label}
            </button>
          ))}
        </div>
      </details>

      <label className="dm-chain-toggle">
        <input
          type="checkbox"
          checked={chainEnabled}
          disabled={disabled || !stopped}
          onChange={(event) => onChainEnabledChange(event.target.checked)}
        />
        CHAIN
      </label>

      <details className="dm-chain-editor">
        <summary className="dm-bank-tool">EDIT CHAIN</summary>
        <div className="dm-chain-panel">
          <ol aria-label="Pattern chain">
            {Array.from(chain, (bank, index) => (
              <li
                key={index}
                data-active={
                  chainEnabled && index === chainPosition ? true : undefined
                }
              >
                <span>{index + 1}</span>
                <select
                  aria-label={`Chain entry ${index + 1}`}
                  value={bank}
                  disabled={disabled || !stopped}
                  onChange={(event) =>
                    updateEntry(index, Number(event.target.value))
                  }
                >
                  {BANK_LABELS.map((label, value) => (
                    <option key={label} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  disabled={disabled || !stopped || chain.length <= 1}
                  onClick={() => removeEntry(index)}
                  aria-label={`Remove chain entry ${index + 1}`}
                >
                  ×
                </button>
              </li>
            ))}
          </ol>
          <button
            type="button"
            disabled={disabled || !stopped || chain.length >= MAX_CHAIN_LENGTH}
            onClick={() =>
              onChainChange(
                Uint8Array.from([...chain, chain[chain.length - 1] ?? 0]),
              )
            }
          >
            ADD ENTRY
          </button>
          {!stopped && <small>Stop playback to edit the chain.</small>}
        </div>
      </details>

      <span className="dm-bank-status" role="status" aria-live="polite">
        {status}
      </span>
    </section>
  );
}
