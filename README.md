# Kafka Hello World

This application was my "Kafka Hello World". 

It initializes Kafka with 1 exchange and 2 queues: for tasks and for results. 

The client side accepts a command from stdin and sends it to the tasks queue, concurrently listening for the results queue.

Server side listens for the tasks queue, calculates expression (task is a simple arithmetical expression) and sends it to the results queue. 

