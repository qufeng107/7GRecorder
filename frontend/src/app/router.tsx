import { Suspense, lazy } from "react";
import { createBrowserRouter, Navigate } from "react-router-dom";
import { FeatureGate } from "./FeatureGate";
const ConsoleLayout = lazy(() => import("./ConsoleLayout"));
const PublicStreamer = lazy(() =>
  import("../pages/PublicStreamer").then((m) => ({
    default: m.PublicStreamer,
  })),
);
const Overview = lazy(() => import("../features/overview/OverviewPage"));
const Profiles = lazy(() => import("../features/profiles/ProfilesPage"));
const Recordings = lazy(() => import("../features/recordings/RecordingsPage"));
const RecordingDetail = lazy(() => import("../features/recordings/RecordingDetailPage"));
const Uploads = lazy(() => import("../features/uploads/UploadsPage"));
const Songs = lazy(() => import("../features/songs/SongsPage"));
const Jobs = lazy(() => import("../features/jobs/JobsPage"));
const CaptureSession = lazy(() => import("../features/liveAnalytics/CaptureSessionPage"));
const LiveAnalytics = lazy(() => import("../features/liveAnalytics/LiveAnalyticsPage"));
const System = lazy(() => import("../features/system/SystemPage"));
const Accounts = lazy(() => import("../features/accounts/AccountsPage"));
const Me = lazy(() => import("../features/accounts/MePage"));
const pending = (
  <p role="status" className="p-8">
    正在加载…
  </p>
);
export const router = createBrowserRouter([
  { path: "/", element: <Navigate to="/admin" replace /> },
  {
    path: "/admin",
    element: (
      <Suspense fallback={pending}>
        <ConsoleLayout />
      </Suspense>
    ),
    children: [
      {
        index: true,
        element: (
          <FeatureGate>
            <Suspense fallback={pending}>
              <Overview />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "profiles",
        element: (
          <FeatureGate>
            <Suspense fallback={pending}>
              <Profiles />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "recordings",
        element: (
          <FeatureGate>
            <Suspense fallback={pending}>
              <Recordings />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "recordings/source/:sourceId",
        element: <FeatureGate><Suspense fallback={pending}><RecordingDetail /></Suspense></FeatureGate>,
      },
      {
        path: "uploads",
        element: (
          <FeatureGate access="uploads">
            <Suspense fallback={pending}>
              <Uploads />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "songs",
        element: (
          <FeatureGate access="admin">
            <Suspense fallback={pending}>
              <Songs />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "live-analytics/sessions/:sessionId",
        element: <FeatureGate><Suspense fallback={pending}><CaptureSession /></Suspense></FeatureGate>,
      },
      {
        path: "live-analytics",
        element: (
          <FeatureGate access="admin">
            <Suspense fallback={pending}><LiveAnalytics /></Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "jobs",
        element: (
          <FeatureGate>
            <Suspense fallback={pending}>
              <Jobs />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "system",
        element: (
          <FeatureGate access="admin">
            <Suspense fallback={pending}>
              <System />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "accounts",
        element: (
          <FeatureGate access="admin">
            <Suspense fallback={pending}>
              <Accounts />
            </Suspense>
          </FeatureGate>
        ),
      },
      {
        path: "me",
        element: (
          <FeatureGate>
            <Suspense fallback={pending}>
              <Me />
            </Suspense>
          </FeatureGate>
        ),
      },
      { path: "*", element: <p>页面不存在 · Page not found</p> },
    ],
  },
  {
    path: "/:handle/*",
    element: (
      <Suspense fallback={pending}>
        <PublicStreamer />
      </Suspense>
    ),
  },
]);
