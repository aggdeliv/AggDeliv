from concurrent import futures
from threading import Thread

import grpc

import controller
import demo_pb2
import demo_pb2_grpc
from controller import agent

import multiprocessing as mp

__all__ = 'DemoServer'
SERVER_ADDRESS = 'localhost:23333'
SERVER_ID = 1
PATH_NUM = 2
SMP_NUM = 10
agg_thp_queue = mp.Queue(1)
smp_queue = mp.Queue(1)
thp_queues = [mp.Queue(SMP_NUM) for p in range(PATH_NUM)]
prob_queues = [mp.Queue(1) for q in range(PATH_NUM)]
lock = mp.Lock()


class DemoServer(demo_pb2_grpc.GRPCDemoServicer):

    def SimpleMethod(self, request, context):
        print("SimpleMethod called by client(%d) the message: %s" %
              (request.client_id, request.request_data))

        info = str(request.request_data).split()
        if info[0] == 'D':
            agg_thp_queue.put(float(info[1])/1000000)
            res_data = ''
            for i in range(PATH_NUM):
                res_data += str(prob_queues[i].get())
                if i < PATH_NUM - 1:
                    res_data += '&'
            print('Link prob: ' + str(res_data))
        else:
            lock.acquire()
            for i in range(PATH_NUM):
                if thp_queues[i].full():
                    thp_queues[i].get()
                thp_queues[i].put(float(info[i+1])/1000000)
            lock.release()
            res_data = str(thp_queues[0].qsize())

        response = demo_pb2.Response(
            server_id=SERVER_ID,
            response_data=str(res_data))
        return response

    def ClientStreamingMethod(self, request_iterator, context):
        print("ClientStreamingMethod called by client...")
        for request in request_iterator:
            print("recv from client(%d), message= %s" %
                  (request.client_id, request.request_data))
        response = demo_pb2.Response(
            server_id=SERVER_ID,
            response_data="Python server ClientStreamingMethod ok")
        return response

    def ServerStreamingMethod(self, request, context):
        print("ServerStreamingMethod called by client(%d), message= %s" %
              (request.client_id, request.request_data))

        def response_messages():
            for i in range(5):
                response = demo_pb2.Response(
                    server_id=SERVER_ID,
                    response_data=("send by Python server, message=%d" % i))
                yield response

        return response_messages()

    def BidirectionalStreamingMethod(self, request_iterator, context):
        print("BidirectionalStreamingMethod called by client...")

        def parse_request():
            for request in request_iterator:
                print("recv from client(%d), message= %s" %
                      (request.client_id, request.request_data))

        t = Thread(target=parse_request)
        t.start()

        for i in range(5):
            yield demo_pb2.Response(
                server_id=SERVER_ID,
                response_data=("send by Python server, message= %d" % i))

        t.join()


def main():
    rpc_server = grpc.server(futures.ThreadPoolExecutor())
    demo_pb2_grpc.add_GRPCDemoServicer_to_server(DemoServer(), rpc_server)
    rpc_server.add_insecure_port(SERVER_ADDRESS)
    print("------------------start Python GRPC server")
    smp_queue.put(SMP_NUM)
    controller = mp.Process(target=agent, args=(agg_thp_queue, smp_queue, thp_queues, prob_queues, lock))
    controller.start()
    rpc_server.start()
    rpc_server.wait_for_termination()


if __name__ == '__main__':
    main()
