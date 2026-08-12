import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type CSSProperties,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";
import { Icon, type IconName } from "./icons";

/* -------------------------------------------------------------------------- */
/* VisuallyHidden                                                             */
/* -------------------------------------------------------------------------- */

export function VisuallyHidden({ children }: { children: ReactNode }) {
  return <span className="house-visually-hidden">{children}</span>;
}

/* -------------------------------------------------------------------------- */
/* Button / IconButton                                                        */
/* -------------------------------------------------------------------------- */

export type ButtonVariant = "primary" | "secondary" | "ghost" | "destructive";
export type ButtonSize = "default" | "sm";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: IconName;
  iconPosition?: "start" | "end";
  loading?: boolean;
}

const buttonBase: CSSProperties = {
  minHeight: "var(--house-control-height)",
  padding: "0 var(--house-space-4)",
  borderRadius: "var(--house-radius-md)",
  font: "500 var(--house-type-control) / var(--house-leading-control) var(--house-font-product)",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "var(--house-space-2)",
  cursor: "pointer",
  borderStyle: "solid",
  borderWidth: 1,
  transition: "background-color var(--house-motion-enter), border-color var(--house-motion-enter), color var(--house-motion-enter)",
};

const variantStyle: Record<ButtonVariant, CSSProperties> = {
  primary: {
    borderColor: "var(--house-accent)",
    background: "var(--house-accent)",
    color: "var(--house-accent-ink)",
  },
  secondary: {
    borderColor: "var(--house-rule-strong)",
    background: "transparent",
    color: "var(--house-text-secondary)",
  },
  ghost: {
    borderColor: "transparent",
    background: "transparent",
    color: "var(--house-text-secondary)",
  },
  destructive: {
    borderColor: "var(--house-status-danger)",
    background: "transparent",
    color: "var(--house-status-danger)",
  },
};

export function Button({
  variant = "secondary",
  size = "default",
  icon,
  iconPosition = "start",
  loading = false,
  disabled,
  children,
  style,
  type = "button",
  ...rest
}: ButtonProps) {
  const isDisabled = disabled || loading;
  const pad =
    size === "sm"
      ? { padding: "0 var(--house-space-3)", minHeight: "28px" }
      : {};
  return (
    <button
      type={type}
      disabled={isDisabled}
      aria-busy={loading || undefined}
      style={{
        ...buttonBase,
        ...variantStyle[variant],
        ...pad,
        opacity: isDisabled ? 0.55 : 1,
        cursor: isDisabled ? "not-allowed" : "pointer",
        ...style,
      }}
      {...rest}
    >
      {icon && iconPosition === "start" ? <Icon name={icon} size="compact" /> : null}
      {children}
      {icon && iconPosition === "end" ? <Icon name={icon} size="compact" /> : null}
    </button>
  );
}

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: IconName;
  /** Accessible name — required for icon-only controls. */
  label: string;
  variant?: ButtonVariant;
}

export function IconButton({
  icon,
  label,
  variant = "ghost",
  disabled,
  style,
  type = "button",
  ...rest
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      disabled={disabled}
      style={{
        ...buttonBase,
        ...variantStyle[variant],
        width: "var(--house-control-height)",
        minWidth: "var(--house-control-height)",
        padding: 0,
        opacity: disabled ? 0.55 : 1,
        cursor: disabled ? "not-allowed" : "pointer",
        ...style,
      }}
      {...rest}
    >
      <Icon name={icon} size="default" />
      <VisuallyHidden>{label}</VisuallyHidden>
    </button>
  );
}

/* -------------------------------------------------------------------------- */
/* Field                                                                      */
/* -------------------------------------------------------------------------- */

type FieldControlProps =
  | ({ as?: "input" } & InputHTMLAttributes<HTMLInputElement>)
  | ({ as: "textarea" } & TextareaHTMLAttributes<HTMLTextAreaElement>)
  | ({ as: "select"; children: ReactNode } & SelectHTMLAttributes<HTMLSelectElement>);

export type FieldProps = {
  label: string;
  hint?: string;
  error?: string;
  required?: boolean;
  id?: string;
  className?: string;
  style?: CSSProperties;
} & FieldControlProps;

const controlStyle: CSSProperties = {
  width: "100%",
  minHeight: "var(--house-control-height)",
  padding: "0 var(--house-space-3)",
  borderRadius: "var(--house-radius-md)",
  border: "1px solid var(--house-rule-strong)",
  background: "var(--house-bg-sunken)",
  color: "var(--house-text-primary)",
  font: "400 var(--house-type-body) / var(--house-leading-body) var(--house-font-product)",
};

export function Field(props: FieldProps) {
  const genId = useId();
  const {
    label,
    hint,
    error,
    required,
    id: idProp,
    className,
    style,
    as = "input",
    ...controlProps
  } = props as FieldProps & { as?: string };
  const id = idProp ?? genId;
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;

  const shared = {
    id,
    required,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": describedBy,
    style: {
      ...controlStyle,
      borderColor: error ? "var(--house-status-danger)" : "var(--house-rule-strong)",
      ...(as === "textarea" ? { minHeight: 96, padding: "var(--house-space-2) var(--house-space-3)", resize: "vertical" as const } : {}),
    },
  };

  let control: ReactNode;
  if (as === "textarea") {
    control = (
      <textarea
        {...(controlProps as TextareaHTMLAttributes<HTMLTextAreaElement>)}
        {...shared}
      />
    );
  } else if (as === "select") {
    control = (
      <select
        {...(controlProps as SelectHTMLAttributes<HTMLSelectElement>)}
        {...shared}
      />
    );
  } else {
    control = (
      <input
        {...(controlProps as InputHTMLAttributes<HTMLInputElement>)}
        {...shared}
      />
    );
  }

  return (
    <div
      className={className}
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--house-space-1)",
        ...style,
      }}
    >
      <label
        htmlFor={id}
        style={{
          font: "500 var(--house-type-label) / 1.3 var(--house-font-product)",
          color: "var(--house-text-secondary)",
        }}
      >
        {label}
        {required ? (
          <span aria-hidden="true" style={{ color: "var(--house-status-danger)" }}>
            {" "}
            *
          </span>
        ) : null}
      </label>
      {control}
      {hint && !error ? (
        <span
          id={hintId}
          style={{
            font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
            color: "var(--house-text-tertiary)",
          }}
        >
          {hint}
        </span>
      ) : null}
      {error ? (
        <span
          id={errorId}
          role="alert"
          style={{
            font: "500 var(--house-type-metadata) / 1.3 var(--house-font-technical)",
            color: "var(--house-status-danger)",
          }}
        >
          {error}
        </span>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Status                                                                     */
/* -------------------------------------------------------------------------- */

export type StatusTone = "ok" | "warning" | "danger" | "info" | "neutral";

const statusIcon: Record<StatusTone, IconName | null> = {
  ok: "check",
  warning: "alert",
  danger: "error",
  info: "info",
  neutral: null,
};

const statusColor: Record<StatusTone, string> = {
  ok: "var(--house-status-ok)",
  warning: "var(--house-status-warning)",
  danger: "var(--house-status-danger)",
  info: "var(--house-status-info)",
  neutral: "var(--house-text-tertiary)",
};

const statusBg: Record<StatusTone, string> = {
  ok: "var(--house-status-ok-bg)",
  warning: "var(--house-status-warning-bg)",
  danger: "var(--house-status-danger-bg)",
  info: "var(--house-status-info-bg)",
  neutral: "var(--house-bg-raised)",
};

export interface StatusProps {
  tone?: StatusTone;
  children: ReactNode;
  /** When true, omit the leading icon (text+color only — prefer icon+text). */
  hideIcon?: boolean;
  className?: string;
  style?: CSSProperties;
}

export function Status({
  tone = "neutral",
  children,
  hideIcon = false,
  className,
  style,
}: StatusProps) {
  const icon = hideIcon ? null : statusIcon[tone];
  return (
    <span
      className={className}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--house-space-2)",
        border: "1px solid currentColor",
        borderRadius: "var(--house-radius-sm)",
        padding: "var(--house-space-1) var(--house-space-2)",
        font: "500 var(--house-type-metadata) / 1.3 var(--house-font-technical)",
        color: statusColor[tone],
        background: statusBg[tone],
        ...style,
      }}
    >
      {icon ? <Icon name={icon} size="compact" /> : null}
      {children}
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* PageHeader                                                                 */
/* -------------------------------------------------------------------------- */

export interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  lede?: string;
  meta?: ReactNode;
  actions?: ReactNode;
  className?: string;
  style?: CSSProperties;
}

export function PageHeader({
  eyebrow,
  title,
  lede,
  meta,
  actions,
  className,
  style,
}: PageHeaderProps) {
  return (
    <header
      className={className}
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--house-space-2)",
        marginBottom: "var(--house-space-6)",
        ...style,
      }}
    >
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: "var(--house-space-3)",
        }}
      >
        <div style={{ minWidth: 0, flex: "1 1 auto" }}>
          {eyebrow ? <div className="house-type-eyebrow">{eyebrow}</div> : null}
          <h1 className="house-type-title" style={{ margin: "var(--house-space-1) 0 0" }}>
            {title}
          </h1>
          {lede ? (
            <p className="house-type-lede" style={{ margin: "var(--house-space-2) 0 0" }}>
              {lede}
            </p>
          ) : null}
        </div>
        {actions ? (
          <div
            style={{
              display: "flex",
              flexWrap: "wrap",
              gap: "var(--house-space-2)",
              flex: "0 0 auto",
            }}
          >
            {actions}
          </div>
        ) : null}
      </div>
      {meta ? (
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--house-space-3)",
            paddingTop: "var(--house-space-3)",
            borderTop: "1px solid var(--house-rule-subtle)",
            color: "var(--house-text-tertiary)",
            font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
          }}
        >
          {meta}
        </div>
      ) : null}
    </header>
  );
}

/* -------------------------------------------------------------------------- */
/* DataState                                                                  */
/* -------------------------------------------------------------------------- */

export type DataStateKind = "loading" | "empty" | "error" | "denied" | "stale";

export interface DataStateProps {
  kind: DataStateKind;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
  style?: CSSProperties;
}

const dataStateIcon: Record<DataStateKind, IconName> = {
  loading: "refresh",
  empty: "search",
  error: "error",
  denied: "access",
  stale: "clock",
};

const dataStateTone: Record<DataStateKind, StatusTone> = {
  loading: "info",
  empty: "neutral",
  error: "danger",
  denied: "warning",
  stale: "warning",
};

export function DataState({
  kind,
  title,
  description,
  action,
  className,
  style,
}: DataStateProps) {
  return (
    <div
      className={className}
      role={kind === "error" || kind === "denied" ? "alert" : "status"}
      aria-live={kind === "loading" ? "polite" : undefined}
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "flex-start",
        gap: "var(--house-space-3)",
        padding: "var(--house-space-6)",
        border: "1px solid var(--house-rule-subtle)",
        borderRadius: "var(--house-radius-md)",
        background: "var(--house-bg-surface)",
        ...style,
      }}
    >
      <Status tone={dataStateTone[kind]}>
        <Icon name={dataStateIcon[kind]} size="compact" />
        {kind}
      </Status>
      <div>
        <div className="house-type-section">{title}</div>
        {description ? (
          <p className="house-type-lede" style={{ margin: "var(--house-space-1) 0 0" }}>
            {description}
          </p>
        ) : null}
      </div>
      {action}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Modal / Drawer                                                             */
/* -------------------------------------------------------------------------- */

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  /** "modal" centers; "drawer" docks to the end edge. */
  variant?: "modal" | "drawer";
  className?: string;
}

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  variant = "modal",
  className,
}: ModalProps) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    previouslyFocused.current = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    const focusable = panel?.querySelector<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    focusable?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key !== "Tab" || !panel) return;
      const nodes = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled") && el.tabIndex !== -1);
      if (nodes.length === 0) return;
      const first = nodes[0]!;
      const last = nodes[nodes.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      previouslyFocused.current?.focus?.();
    };
  }, [open, onClose]);

  if (!open) return null;

  const isDrawer = variant === "drawer";

  return (
    <div
      role="presentation"
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 50,
        display: "flex",
        alignItems: isDrawer ? "stretch" : "center",
        justifyContent: isDrawer ? "flex-end" : "center",
        background: "rgb(0 0 0 / 55%)",
        padding: isDrawer ? 0 : "var(--house-space-4)",
      }}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={["house-modal", className].filter(Boolean).join(" ")}
        style={{
          background: "var(--house-bg-surface)",
          color: "var(--house-text-primary)",
          border: "1px solid var(--house-rule-subtle)",
          borderRadius: isDrawer ? 0 : "var(--house-radius-md)",
          width: isDrawer ? "min(420px, 100%)" : "min(480px, 100%)",
          maxHeight: isDrawer ? "100%" : "min(90vh, 720px)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "var(--house-space-3)",
            padding: "var(--house-space-4)",
            borderBottom: "1px solid var(--house-rule-subtle)",
          }}
        >
          <h2 id={titleId} className="house-type-section" style={{ margin: 0 }}>
            {title}
          </h2>
          <IconButton icon="close" label="Close" onClick={onClose} />
        </div>
        <div
          style={{
            padding: "var(--house-space-4)",
            overflow: "auto",
            flex: "1 1 auto",
          }}
        >
          {children}
        </div>
        {footer ? (
          <div
            style={{
              display: "flex",
              justifyContent: "flex-end",
              gap: "var(--house-space-2)",
              padding: "var(--house-space-4)",
              borderTop: "1px solid var(--house-rule-subtle)",
            }}
          >
            {footer}
          </div>
        ) : null}
      </div>
    </div>
  );
}

export const Drawer = (props: Omit<ModalProps, "variant">) => (
  <Modal {...props} variant="drawer" />
);

/* -------------------------------------------------------------------------- */
/* CopyableId                                                                 */
/* -------------------------------------------------------------------------- */

export interface CopyableIdProps {
  value: string;
  /** Visible truncated form; defaults to first 8…last 4. */
  display?: string;
  label?: string;
  className?: string;
  style?: CSSProperties;
}

function defaultTruncate(value: string): string {
  if (value.length <= 14) return value;
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

export function CopyableId({
  value,
  display,
  label = "Copy ID",
  className,
  style,
}: CopyableIdProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const onCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be unavailable; leave state unchanged.
    }
  }, [value]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  return (
    <span
      className={className}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--house-space-1)",
        font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
        color: "var(--house-text-tertiary)",
        ...style,
      }}
    >
      <code
        title={value}
        style={{
          font: "inherit",
          background: "transparent",
          color: "inherit",
        }}
      >
        {display ?? defaultTruncate(value)}
      </code>
      <IconButton
        icon={copied ? "check" : "copy"}
        label={copied ? "Copied" : label}
        onClick={() => void onCopy()}
        style={{
          width: 28,
          minWidth: 28,
          minHeight: 28,
          color: copied ? "var(--house-status-ok)" : undefined,
        }}
      />
    </span>
  );
}
