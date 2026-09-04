import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AdminDashboard } from "./AdminDashboard";

describe("AdminDashboard", () => {
  it("renders the bootstrap console", () => {
    render(<AdminDashboard />);
    expect(screen.getByText("录播平台控制台")).toBeInTheDocument();
    expect(screen.getByText("Local Storage")).toBeInTheDocument();
  });
});
