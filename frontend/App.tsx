import {
  createSignal,
  type Component,
  onMount,
  createResource,
  Resource,
} from "solid-js";
import { Chart, Title, Tooltip, Legend, Colors, TimeScale } from "chart.js";
import { Line } from "solid-chartjs";
import { type ChartData, type ChartOptions } from "chart.js";
import colors from "tailwindcss/colors";
import 'chartjs-adapter-date-fns';

import icon from "../assets/favicon.png";

// Get the URL from the import.meta object
const host = import.meta.env.VITE_API_HOST;

const mapData = (data: any) => {
  const mapped = data?.map(({ time, count }: any) => ({ x: time, y: count })).slice(data.length - 24, data.length)?? []
  return mapped
};

const PostPerHourChart: Component<{ data: Resource<any> }> = ({ data }) => {
  /**
   * You must register optional elements before using the chart,
   * otherwise you will have the most primitive UI
   */
  onMount(() => {
    Chart.register(Title, Tooltip, Legend, Colors, TimeScale);
  });

  const chartData: ChartData = {
    datasets: [
      {
        label: "Posts per hour",
        borderColor: colors.blue[500],
        fill: false,
        data: [],
        tension: 0.2,
      },
    ],
  };

  const chartOptions: ChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        type: 'time',
        time: {
          // Luxon format string
          tooltipFormat: 'hh:mm',
          displayFormats: {
            hour: 'E  hh:mm'
          }
        },
        title: {
          display: true,
          text: 'Date',
          color: colors.zinc[400],
        },
        adapters: {
          date: {
          }
        },
        grid: {
          color: colors.zinc[800],
        },
        ticks: {
          color: colors.zinc[400],
          
        },
      },
      y: {
        title: {
          display: true,
          text: 'Count',
          color: colors.zinc[400],
        },
        grid: {
          color: colors.zinc[800],
        },
        ticks: {
          color: colors.zinc[400],
        }
      }
    },
    plugins: {
      legend: {
          display: false,
      }
  }

  };

  return (
    <div>
      <Line
        data={{
          ...chartData,
          datasets: [
            {
              ...chartData.datasets[0],
              data: data.state === 'ready' ? mapData(data()) : [],
            },
          ],
        }}
        options={chartOptions}
        width={500}
        height={200}
        
      />
    </div>
  );
};

const fetcher = (url: string) => fetch(url).then((res) => res.json());

const PostPerHour: Component<{ lang: string, label: string }> = ({ lang, label }) => {
  // Create a new resource signal to fetch data from the API
  // That is createResource('http://localhost:3000/dashboard/posts-per-hour');
  const [data] = createResource(
    `${host}/dashboard/posts-per-hour?lang=${lang}`,
    fetcher
  );
  return (
    <div>
      <h1 class="text-2xl text-sky-300 text-center pb-8">{label}</h1>
      <PostPerHourChart data={data} />
    </div>
  );
};

const App: Component = () => {

  return (
    <div class="flex flex-col p-16 gap-16">
      {/* Add a header here showing the Norsky logo and the name */}
      <div class="flex justify-start items-center gap-4">
        <img src={icon} alt="Norsky logo" class="w-16 h-16" />
        <h1 class="text-4xl text-sky-300">Norsky</h1>
      </div>
      <div class="flex">
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-16 w-full">
          <PostPerHour lang="" label="All languages"/>
          <PostPerHour lang="nb" label="Norwegian bokmål"/>
          <PostPerHour lang="nn" label="Norwegian nynorsk"/>
          <PostPerHour lang="smi" label="Sami"/>
        </div>
      </div>
    </div>
  );
};

export default App;
