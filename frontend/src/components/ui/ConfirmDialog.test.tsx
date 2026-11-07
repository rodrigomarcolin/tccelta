import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

type Props = Partial<ComponentProps<typeof ConfirmDialog>>;

function setup(props: Props = {}) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const ui = (p: Props) => (
    <ConfirmDialog
      open
      title="Excluir a viagem de Maria?"
      confirmLabel="Excluir"
      pendingLabel="Excluindo…"
      onConfirm={onConfirm}
      onCancel={onCancel}
      {...p}
    >
      Não é possível desfazer.
    </ConfirmDialog>
  );
  const view = render(ui(props));
  return { onConfirm, onCancel, rerender: (p: Props) => view.rerender(ui(p)), view };
}

describe("ConfirmDialog", () => {
  it("opens as a modal, named by its title and described by its text", () => {
    setup();
    const dialog = screen.getByRole("dialog", { name: "Excluir a viagem de Maria?" });
    expect(dialog).toHaveAttribute("open");
    expect(dialog).toHaveAccessibleDescription("Não é possível desfazer.");
  });

  it("starts with the focus on Cancelar", () => {
    setup();
    expect(screen.getByRole("button", { name: "Cancelar" })).toHaveFocus();
  });

  it("is closed while open is false, and follows it", () => {
    const { rerender } = setup({ open: false });
    expect(screen.queryByRole("dialog")).toBeNull();
    rerender({ open: true });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    rerender({ open: false });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("gives the focus back to the control that opened it", () => {
    const opener = document.createElement("button");
    document.body.append(opener);
    const { rerender } = setup({ open: false });
    opener.focus();
    rerender({ open: true });
    expect(screen.getByRole("button", { name: "Cancelar" })).toHaveFocus();
    rerender({ open: false });
    expect(opener).toHaveFocus();
    opener.remove();
  });

  it("Esc (the cancel event) asks to cancel and does not close by itself", () => {
    const { onCancel } = setup();
    const dialog = screen.getByRole("dialog");
    const event = new Event("cancel", { cancelable: true });
    dialog.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(onCancel).toHaveBeenCalledOnce();
    expect(dialog).toHaveAttribute("open");
  });

  it("the buttons call back", async () => {
    const { onConfirm, onCancel } = setup();
    await userEvent.click(screen.getByRole("button", { name: "Excluir" }));
    expect(onConfirm).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("a click on the backdrop cancels, a click inside does not", () => {
    const { onCancel } = setup();
    fireEvent.click(screen.getByText("Não é possível desfazer."));
    expect(onCancel).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("dialog"));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("while pending: both buttons disabled, Excluindo…, and Esc or the backdrop do nothing", () => {
    const { onCancel, onConfirm } = setup({ pending: true });
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
    const confirm = screen.getByRole("button", { name: "Excluindo…" });
    expect(confirm).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Excluir" })).toBeNull();
    const event = new Event("cancel", { cancelable: true });
    screen.getByRole("dialog").dispatchEvent(event);
    fireEvent.click(screen.getByRole("dialog"));
    fireEvent.click(confirm);
    expect(event.defaultPrevented).toBe(true);
    expect(onCancel).not.toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("an error stays inside the open dialog, as an alert", () => {
    setup({ error: "Não foi possível excluir. Tente de novo." });
    expect(screen.getByRole("alert")).toHaveTextContent("Não foi possível excluir. Tente de novo.");
    expect(screen.getByRole("dialog")).toHaveAttribute("open");
    expect(screen.getByRole("button", { name: "Excluir" })).toBeEnabled();
  });

  it("without an error there is no alert", () => {
    setup();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("unmounting closes it, so the page is not left inert", () => {
    const { view } = setup();
    const dialog = screen.getByRole("dialog");
    view.unmount();
    expect(dialog).not.toHaveAttribute("open");
  });
});
