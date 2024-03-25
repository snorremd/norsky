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
} from "solid-js";
import { Chart, Title, Tooltip, Legend, Colors, TimeScale, ChartDataset, ChartType, Point, TimeUnit, TimeSeriesScale } from "chart.js";
import { Line } from "solid-chartjs";
import { type ChartData, type ChartOptions } from "chart.js";
import { formatRelative } from 'date-fns'
import colors from "tailwindcss/colors";
import "chartjs-adapter-date-fns";

import icon from "../assets/favicon.png";

// Get the URL from the import.meta object
const host = import.meta.env.VITE_API_HOST;


interface Data {
  time: string,
  count: number
}


const mapData = (data: Data[]): Point[] => {
  const mapped =
    data
      ?.map(({ time, count }: any) => ({ x: time, y: count }))
      .slice(data.length - 24, data.length) ?? [];
  return mapped;
};


interface ChartDataProps {
  data: Point[],
  time: string
}

const chartData = ({data, time}: ChartDataProps) => {
  interface ChartData {
    datasets: ChartDataset[],
    labels: string[]
  }

  const chartData: ChartData = {
    datasets: [
      {
        borderColor: colors.blue[500],
        fill: false,
        tension: 0.2,
        label: `Posts per ${time}`,
        data,
        type: 'line',
      },
    ],
    labels: []
  };

  return chartData;
}

const timeToUnit = (time: string): TimeUnit => {
  switch (time) {
    case "hour":
      return 'hour'
    case "day":
      return 'day'
    case "week":
      return 'week'
    default:
      return 'hour'
  }
}

const chartOptions = ({time}: {time: string}) => {

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

  console.log(options)
  return options;
}


const PostPerHourChart: Component<{ data: Resource<Data[]>, time: Accessor<string> }> = ({ data, time }) => {
  /**
   * You must register optional elements before using the chart,
   * otherwise you will have the most primitive UI
   */
  onMount(() => {
    Chart.register(Title, Tooltip, Legend, Colors, TimeScale, TimeSeriesScale);
  });

  const cdata = () => chartData({data: mapData(data()), time: time()})
  const coptions = createMemo(() => chartOptions({time: time()}))


  return (
    <div class="flex flex-col">

      <Line
        data={cdata()}
        options={coptions()}
        width={500}
        height={300}
      />
    </div>
  );
};

const fetcher = ([time, lang]: readonly string[]) =>
  fetch(`${host}/dashboard/posts-per-time?lang=${lang}&time=${time}`).then(
    (res) => res.json() as Promise<Data[]>
  );

const PostPerTime: Component<{
  lang: string;
  label: string;
  time: Accessor<string>;
}> = ({ lang, label, time }) => {
  // Create a new resource signal to fetch data from the API
  // That is createResource('http://localhost:3000/dashboard/posts-per-hour');

  const [data] = createResource(() => [time(), lang] as const, fetcher);
  return (
    <div class="h-full">
      <h1 class="text-2xl text-sky-300 text-center pb-8">{label}</h1>
      <PostPerHourChart time={time} data={data} />
    </div>
  );
};

interface Post {
  createdAt: number,
  languages: string[]
  text: string
  uri: string
}

const langToName = (lang: string): string => {
  switch (lang) {
    case "nb":
      return "Norwegian bokmål"
    case "nn":
      return "Norwegian nynorsk"
    case "se":
      return "Northern Sami"
    default:
      return lang
  }
}

const PostFirehose: Component = () => {
  const [key, setKey] = createSignal<string>(); // Used to politely close the event source
  const [posts, setPosts] = createSignal<Post[]>([]);
  const [eventSource, setEventSource] = createSignal<EventSource | null>(null);

  onMount(() => {
    console.log("Mounting event source")
    const es = new EventSource(`${host}/dashboard/feed/sse`);
    setEventSource(es);

    es.onmessage = (e) => {
      if(key() === undefined) {
        setKey(e.data);
        return;
      }
      console.log("Message received", e);
      const post = JSON.parse(e.data) as Post;
      setPosts((posts) => [post, ...posts.slice(0, 499)]); // Limit to 500 posts
    };
  });

  const close = async () => {
    console.log("Closing event source");
    eventSource()?.close();
    await fetch(`${host}/dashboard/feed/sse?key=${key()}`, { method: "DELETE" })
  }

  if (import.meta.hot) {
    import.meta.hot.accept(close);
  }

  window.addEventListener("beforeunload", close)


  // Display a pretty list of the posts
  // Set a max height and use overflow-y: scroll to make it scrollable
  // Height should be whatever the parent is.

  return <div class="flex flex-col gap-4 h-[800px] max-h-[65vh] col-span-full md:col-span-2">
    <h1 class="text-2xl text-sky-300 text-center pb-8">Recent posts</h1>
    <div class="overflow-y-scroll scroll h-full gap-4 flex flex-col no-scrollbar bg-zinc-800 rounded-lg p-4">
    <For each={posts()}>
      {(post) => {
        const createdAt = formatRelative(new Date(post.createdAt * 1000), new Date())
        // Match regex to get the profile and post id
        // URI example: at://did:plc:opkjeuzx2lego6a7gueytryu/app.bsky.feed.post/3kcbxsslpu623
        // profile = did:plc:opkjeuzx2lego6a7gueytryu
        // post = 3kcbxsslpu623

        const regex = /at:\/\/(did:plc:[a-z0-9]+)\/app.bsky.feed.post\/([a-z0-9]+)/
        const [profile, postId] = regex.exec(post.uri)!.slice(1)
        const bskyLink = `https://bsky.app/profile/${profile}/post/${postId}`
        return <div class="flex flex-col gap-4 p-4 bg-zinc-900 rounded-md">
          <div class="flex flex-row justify-between">
            <p class="text-zinc-400">{createdAt}</p>
            <p class="text-zinc-400">{post.languages.map(langToName).join(", ")}</p>
          </div>
          <p class="text-zinc-300 w-full max-w-[80ch]">{post.text}</p>

          {/* Link to post on Bluesky */}
          <div class="flex flex-row justify-end">
            <a class="text-sky-300 hover:text-sky-200 underline" href={bskyLink} target="_blank">View on Bsky</a>
          </div>
        </div>
      }}
    </For>
    </div>
  </div>;
}
  


const App: Component = () => {
  const [time, setTime] = createSignal<string>("hour");

  return (
    <div class="flex flex-col p-6 md:p-8 lg:p-16">
      {/* Add a header here showing the Norsky logo and the name */}
      <div class="flex justify-start items-center gap-4">
        <img src={icon} alt="Norsky logo" class="w-16 h-16" />
        <h1 class="text-4xl text-sky-300">Norsky</h1>
      </div>
      <div class="flex flex-col">
        <div class="flex flex-row gap-4 justify-end mb-8">
          {/* Selector to select time level: hour, day, week */}
          <select
              class="bg-zinc-800 text-zinc-300 rounded-md p-2"
              value={time()}
              onChange={(e) => setTime(e.currentTarget.value)}
            >
              <option value="hour">Hour</option>
              <option value="day">Day</option>
              <option value="week">Week</option>
            </select>
        </div>
        <div class="grid grid-cols-1 md:grid-cols-2 2xl:grid-cols-4 gap-16 w-full">
          <PostPerTime time={time} lang="" label="All languages" />
          <PostPerTime time={time} lang="nb" label="Norwegian bokmål" />
          <PostPerTime time={time} lang="nn" label="Norwegian nynorsk" />
          <PostPerTime time={time} lang="se" label="Northern Sami" />
          <PostFirehose />
        </div>
      </div>
    </div>
  );
};

export default App;
