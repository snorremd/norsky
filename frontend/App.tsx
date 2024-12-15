import {
  createSignal,
  type Component,
  onMount,
  createResource,
  Resource,
  Accessor,
  createMemo,
  onCleanup,
  For,
  JSX,
} from "solid-js";
import {
  Chart,
  Title,
  Tooltip,
  Legend,
  Colors,
  TimeScale,
  ChartDataset,
  ChartType,
  Point,
  TimeUnit,
  TimeSeriesScale,
} from "chart.js";
import { Line } from "solid-chartjs";
import { type ChartData, type ChartOptions } from "chart.js";
import { formatRelative } from "date-fns";
import colors from "tailwindcss/colors";
import "chartjs-adapter-date-fns";

import icon from "../assets/favicon.png";

// Get the URL from the import.meta object
const host = import.meta.env.VITE_API_HOST;

interface Data {
  time: string;
  count: number;
}

const mapData = (data: Data[]): Point[] => {
  const mapped =
    data
      ?.map(({ time, count }: any) => ({ x: time, y: count }))
      .slice(data.length - 24, data.length) ?? [];
  return mapped;
};

interface ChartDataProps {
  data: Point[];
  time: string;
}

const chartData = ({ data, time }: ChartDataProps) => {
  interface ChartData {
    datasets: ChartDataset[];
    labels: string[];
  }

  const chartData: ChartData = {
    datasets: [
      {
        borderColor: colors.blue[500],
        fill: false,
        tension: 0.2,
        label: `Posts per ${time}`,
        data,
        type: "line",
      },
    ],
    labels: [],
  };

  return chartData;
};

const timeToUnit = (time: string): TimeUnit => {
  switch (time) {
    case "hour":
      return "hour";
    case "day":
      return "day";
    case "week":
      return "week";
    default:
      return "hour";
  }
};

const chartOptions = ({ time }: { time: string }) => {
  const options: ChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        type: "timeseries",
        time: {
          // Luxon format string
          minUnit: timeToUnit(time),
          displayFormats: {
            hour: "HH:mm",
            day: "EE dd.MM",
            week: "I",
          },
          tooltipFormat: "dd.MM.yyyy HH:mm",
        },
        title: {
          display: true,
          text: "Time",
          color: colors.zinc[400],
        },
        adapters: {
          date: {},
        },
        grid: {
          color: colors.zinc[800],
        },
        ticks: {
          maxRotation: 160,
          color: colors.zinc[400],
        },
      },
      y: {
        title: {
          display: true,
          text: "Count",
          color: colors.zinc[400],
        },
        grid: {
          color: colors.zinc[800],
        },
        ticks: {
          color: colors.zinc[400],
        },
      },
    },
    plugins: {
      legend: {
        display: false,
      },
    },
  };

  console.log(options);
  return options;
};

const PostPerHourChart: Component<{
  data: Resource<Data[]>;
  time: Accessor<string>;
}> = ({ data, time }) => {
  /**
   * You must register optional elements before using the chart,
   * otherwise you will have the most primitive UI
   */
  onMount(() => {
    Chart.register(Title, Tooltip, Legend, Colors, TimeScale, TimeSeriesScale);
  });

  const cdata = () => chartData({ data: mapData(data()), time: time() });
  const coptions = createMemo(() => chartOptions({ time: time() }));

  return (
    <div class="flex flex-col">
      <Line data={cdata()} options={coptions()} width={500} height={300} />
    </div>
  );
};

const fetcher = ([time, lang]: readonly string[]) =>
  fetch(`${host}/dashboard/posts-per-time?lang=${lang}&time=${time}`).then(
    (res) => res.json() as Promise<Data[]>
  );

interface StatWrapper {
  className?: string;
  children: JSX.Element | JSX.Element[];
}

const StatWrapper: Component<StatWrapper> = ({ className, children }) => {
  return (
    <div
      class={`flex flex-col rounded-md border border-zinc-900 p-4 min-h-full bg-zinc-800 ${className}`}
    >
      {children}
    </div>
  );
};

const PostPerTime: Component<{
  lang: string;
  label: string;
  className?: string;
}> = ({ lang, label, className }) => {
  // Create a new resource signal to fetch data from the API
  // That is createResource('http://localhost:3000/dashboard/posts-per-hour');

  const [time, setTime] = createSignal<string>("hour");
  const [data] = createResource(() => [time(), lang] as const, fetcher, {
    initialValue: [],
  });

  return (
    <StatWrapper className={`row-span-2 ${className}`}>
      <div class="flex flex-row justify-between">
        <h2 class="text-xl text-zinc-300 pb-8">{label}</h2>
        <div class="flex flex-row gap-4 justify-end mb-8">
          {/* Radio button to select time level: hour, day, week */}
          <div class="flex flex-row gap-4">
            {["hour", "day", "week"].map((t) => {
              const style =
                time() === t
                  ? "bg-sky-600 text-white"
                  : "bg-gray-700 text-gray-300 hover:bg-gray-600";
              return (
                <label
                  class={`text-xs flex justify-center items-center cursor-pointer px-4 py-0.5 rounded text-center transition
        ${style}`}
                >
                  <input
                    type="radio"
                    name="time"
                    value={t}
                    checked={time() === t}
                    onChange={() => setTime(t)}
                    class="hidden" /* Hides the radio button */
                  />
                  {t}
                </label>
              );
            })}
          </div>
        </div>
      </div>
      <PostPerHourChart time={time} data={data} />
    </StatWrapper>
  );
};

interface Post {
  createdAt: number;
  languages: string[];
  text: string;
  uri: string;
}

const langToName = (lang: string): string => {
  switch (lang) {
    case "nb":
      return "Norwegian bokmål";
    case "nn":
      return "Norwegian nynorsk";
    case "se":
      return "Northern Sami";
    default:
      return lang;
  }
};

interface PostFirehoseProps {
  post: Accessor<Post | undefined>;
  className?: string;
}

const PostFirehose: Component<PostFirehoseProps> = ({ post, className }) => {

  // Match regex to get the profile and post id
  // URI example: at://did:plc:opkjeuzx2lego6a7gueytryu/app.bsky.feed.post/3kcbxsslpu623
  // profile = did:plc:opkjeuzx2lego6a7gueytryu
  // post = 3kcbxsslpu623

  const bskyLink = (post: Post) => {
    const regex = /at:\/\/(did:plc:[a-z0-9]+)\/app.bsky.feed.post\/([a-z0-9]+)/;
    const [profile, postId] = regex.exec(post.uri)!.slice(1);
    return `https://bsky.app/profile/${profile}/post/${postId}`;
  } 

  return (
    <StatWrapper className={`row-span-2 ${className}`}>
      <h1 class="text-2xl text-zinc-300 text-center pb-4">Recent posts</h1>
      <div class="max-h-full gap-4 flex flex-col ">
        {post() ? (
        <div class="flex flex-col gap-4 p-4 bg-zinc-900 rounded-md">
          <div class="flex flex-row justify-between">
            <p class="text-zinc-400">{formatRelative(new Date(post().createdAt * 1000), new Date()) }</p>
            <p class="text-zinc-400">
              {post().languages.map(langToName).join(", ")}
            </p>
          </div>
          <p class="text-zinc-300 w-full max-w-[80ch]">{post().text}</p>

          {/* Link to post on Bluesky */}
          <div class="flex flex-row justify-end">
            <a
              class="text-sky-300 hover:text-sky-200 underline"
              href={bskyLink(post())}
              target="_blank"
            >
              View on Bsky
            </a>
          </div>
        </div>
        ): null}
      </div>
    </StatWrapper>
  );
};

const StatisticStat = ({
  label,
  value,
  className,
}: {
  label: string;
  value: Accessor<number>;
  className?: string;
}) => {
  return (
    <StatWrapper className={`row-span-1 justify-between ${className}`}>
      <h2 class="text-zinc-300 text-xl text-start">{label}</h2>
      <p class="text-sky-300 text-8xl text-center">{value()}</p>
      <p class="text-stone-400 text-sm text-end">per second</p>
    </StatWrapper>
  );
};

const Header = () => {
  return (
    <header
      class={`
      bg-zinc-800
      sticky
      top-0
      flex
      justify-start
      items-center
      gap-4
      px-16
      py-4
    `}
    >
      <img src={icon} alt="Norsky logo" class="w-8 h-8" />
      <h1 class="text-2xl text-zinc-300">Norsky</h1>
    </header>
  );
};

const App: Component = () => {
  const [key, setKey] = createSignal<string>(); // Used to politely close the event source
  const [post, setPost] = createSignal<Post>();
  const [eventsPerSecond, setEventsPerSecond] = createSignal<number>(0);
  const [postsPerSecond, setPostsPerSecond] = createSignal<number>(0);
  const [eventSource, setEventSource] = createSignal<EventSource | null>(null);

  onMount(() => {
    console.log("Mounting event source");
    const setupEventSource = () => {
      const es = new EventSource(`${host}/dashboard/feed/sse`);
      setEventSource(es);

      es.addEventListener("init", (e: MessageEvent) => {
        console.log("Setting key", e.data);
        setKey(e.data);
      });

      es.addEventListener("create-post", (e: MessageEvent) => {
        const data = JSON.parse(e.data);
        console.log("Received post", data);
        setPost(data);
      });

      es.addEventListener("statistics", (e: MessageEvent) => {
        const data = JSON.parse(e.data);
        console.log("Received statistics", data);
        setEventsPerSecond(data.eventsPerSecond);
        setPostsPerSecond(data.postsPerSecond);
      });

      // Add error handling
      es.addEventListener("error", (e) => {
        console.error("EventSource error:", e);
        es.close();
        // Attempt to reconnect after a delay
        setTimeout(setupEventSource, 5000);
      });
    };

    setupEventSource();

    // Cleanup on component unmount
    onCleanup(() => {
      close();
    });
  });

  const close = async () => {
    console.log("Closing event source");
    eventSource()?.close();
    await fetch(`${host}/dashboard/feed/sse?key=${key()}`, {
      method: "DELETE",
    });
  };

  if (import.meta.hot) {
    import.meta.hot.accept(close);
  }

  window.addEventListener("beforeunload", close);

  return (
    <>
      <Header />
      <div class="p-8 min-h-full grid grid-cols-1 md:grid-cols-2 2xl:grid-cols-3 gap-8 w-full">
        <StatisticStat
          className="order-1"
          label="Records"
          value={eventsPerSecond}
        />
        <StatisticStat
          className="order-2 2xl:order-5"
          label="Posts"
          value={postsPerSecond}
        />
        <PostPerTime className="order-3" lang="" label="All languages" />
        <PostPerTime className="order-4" lang="nb" label="Norwegian bokmål" />
        <PostPerTime className="order-5" lang="nn" label="Norwegian nynorsk" />
        <PostPerTime className="order-6" lang="se" label="Northern Sami" />
        <PostFirehose className="order-7" post={post} />
      </div>
    </>
  );
};

export default App;
