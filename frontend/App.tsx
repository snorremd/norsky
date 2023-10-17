import {
  createSignal,
  type Component,
  onMount,
  createResource,
  Resource,
  Accessor,
  createComputed,
  createMemo,
  createEffect,
} from "solid-js";
import { Chart, Title, Tooltip, Legend, Colors, TimeScale, ChartDataset, ChartType, Point, TimeUnit, TimeSeriesScale } from "chart.js";
import { Line } from "solid-chartjs";
import { type ChartData, type ChartOptions } from "chart.js";
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
        height={200}
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
    <div>
      <h1 class="text-2xl text-sky-300 text-center pb-8">{label}</h1>
      <PostPerHourChart time={time} data={data} />
    </div>
  );
};

const App: Component = () => {
  const [time, setTime] = createSignal<string>("hour");

  return (
    <div class="flex flex-col p-6 md:p-8 lg:p-16 gap-16">
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
          <PostPerTime time={time} lang="smi" label="Sami" />
        </div>
      </div>
    </div>
  );
};

export default App;
