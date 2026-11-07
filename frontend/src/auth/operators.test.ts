import { describe, expect, it } from "vitest";
import { accountAllowed, operatorList, safeReturnPath } from "./operators";

const list = "ana@usp.br, Beto@USP.br ,";

describe("accountAllowed", () => {
  it("allows a verified USP e-mail that is in the list", () => {
    expect(accountAllowed({ email: "ana@usp.br", email_verified: true }, list)).toBe(true);
  });
  it("ignores case and spaces in the list and in the e-mail", () => {
    expect(accountAllowed({ email: "BETO@usp.br", email_verified: true }, list)).toBe(true);
    expect(accountAllowed({ email: " ana@usp.br ", email_verified: true }, list)).toBe(true);
  });
  it("allows an e-mail from any domain if it is in the list", () => {
    expect(accountAllowed({ email: "ana@gmail.com", email_verified: true }, "ana@gmail.com")).toBe(
      true,
    );
    expect(
      accountAllowed({ email: "ana@usp.br.evil.com", email_verified: true }, "ana@usp.br.evil.com"),
    ).toBe(true);
  });
  it("rejects an e-mail outside the list, whatever its domain", () => {
    expect(accountAllowed({ email: "carla@usp.br", email_verified: true }, list)).toBe(false);
    expect(accountAllowed({ email: "carla@gmail.com", email_verified: true }, list)).toBe(false);
  });
  it("rejects an unverified e-mail", () => {
    expect(accountAllowed({ email: "ana@usp.br", email_verified: false }, list)).toBe(false);
    expect(accountAllowed({ email: "ana@usp.br" }, list)).toBe(false);
  });
  it("an empty or missing list lets nobody in", () => {
    expect(accountAllowed({ email: "ana@usp.br", email_verified: true }, "")).toBe(false);
    expect(accountAllowed({ email: "ana@usp.br", email_verified: true }, undefined)).toBe(false);
    expect(accountAllowed({ email: "ana@usp.br", email_verified: true }, " , ,")).toBe(false);
  });
  it("rejects a missing profile or one without e-mail", () => {
    expect(accountAllowed(null, list)).toBe(false);
    expect(accountAllowed({ email_verified: true }, list)).toBe(false);
    expect(accountAllowed({ email: "a@b@usp.br", email_verified: true }, "a@b@usp.br")).toBe(false);
  });
});

describe("operatorList", () => {
  it("normalizes", () => {
    expect(operatorList(" A@x , b@y ,,")).toEqual(["a@x", "b@y"]);
    expect(operatorList(null)).toEqual([]);
  });
});

describe("safeReturnPath", () => {
  it("accepts only internal paths", () => {
    expect(safeReturnPath("/viagem/abc")).toBe("/viagem/abc");
    expect(safeReturnPath("//evil.com")).toBe("/processar");
    expect(safeReturnPath("https://evil.com")).toBe("/processar");
    expect(safeReturnPath("/\\evil")).toBe("/processar");
    expect(safeReturnPath(undefined)).toBe("/processar");
  });
});
